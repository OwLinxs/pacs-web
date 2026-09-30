package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/pmfb-saude/pacs-web/backend/internal/units"
	"github.com/pmfb-saude/pacs-web/backend/internal/user"
)

type UnitsStore interface {
	List(context.Context, bool, int, int) ([]units.Unit, error)
	Create(context.Context, string) (units.Unit, error)
	Update(context.Context, uuid.UUID, units.Patch) (units.Unit, units.Unit, error)
}

type unitsPage struct {
	Items      []units.Unit `json:"items"`
	Limit      int          `json:"limit"`
	Offset     int          `json:"offset"`
	HasMore    bool         `json:"hasMore"`
	NextOffset *int         `json:"nextOffset"`
}

func (s *Server) handleListUnits(w http.ResponseWriter, r *http.Request) {
	values, err := url.ParseQuery(r.URL.RawQuery)
	limit, offset, include := units.DefaultLimit, 0, false
	if len(r.URL.RawQuery) > 512 || err != nil {
		s.invalidUnitInput(w)
		return
	}
	for key, values := range values {
		if len(values) != 1 {
			s.invalidUnitInput(w)
			return
		}
		switch key {
		case "includeInactive":
			if values[0] != "true" && values[0] != "false" {
				s.invalidUnitInput(w)
				return
			}
			include = values[0] == "true"
		case "limit", "offset":
			n, err := strconv.Atoi(values[0])
			if err != nil {
				s.invalidUnitInput(w)
				return
			}
			if key == "limit" {
				limit = n
			} else {
				offset = n
			}
		default:
			s.invalidUnitInput(w)
			return
		}
	}
	if limit < 1 || limit > units.MaxLimit || offset < 0 || offset > units.MaxOffset {
		s.invalidUnitInput(w)
		return
	}
	actor, _ := UsuarioDoContexto(r.Context())
	if include && actor.Role != user.RoleAdmin {
		writeError(w, s.log, 403, CodeForbidden, "Somente ADMIN pode consultar unidades inativas.")
		return
	}
	if s.units == nil {
		s.unitError(w, units.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	items, err := s.units.List(ctx, include, limit, offset)
	if err != nil {
		s.unitError(w, err)
		return
	}
	page := unitsPage{Items: items, Limit: limit, Offset: offset, HasMore: len(items) > limit}
	if page.Items == nil {
		page.Items = []units.Unit{}
	}
	if page.HasMore {
		page.Items = items[:limit]
		if next := offset + limit; next <= units.MaxOffset {
			page.NextOffset = &next
		}
	}
	writeJSON(w, s.log, 200, page)
}

func (s *Server) handleCreateUnit(w http.ResponseWriter, r *http.Request) {
	patch, err := decodeUnitInput(w, r, true)
	if err != nil {
		s.unitError(w, err)
		return
	}
	if s.units == nil {
		s.unitError(w, units.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	created, err := s.units.Create(ctx, *patch.Name)
	if err != nil {
		s.unitError(w, err)
		return
	}
	writeJSON(w, s.log, http.StatusCreated, created)
}

func (s *Server) handlePatchUnit(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil || id == uuid.Nil || id.String() != r.PathValue("id") {
		s.invalidUnitInput(w)
		return
	}
	patch, err := decodeUnitInput(w, r, false)
	if err != nil {
		s.unitError(w, err)
		return
	}
	if s.units == nil {
		s.unitError(w, units.ErrUnavailable)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	_, after, err := s.units.Update(ctx, id, patch)
	if err != nil {
		s.unitError(w, err)
		return
	}
	writeJSON(w, s.log, 200, after)
}

func decodeUnitInput(w http.ResponseWriter, r *http.Request, create bool) (units.Patch, error) {
	var patch units.Patch
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	var fields map[string]json.RawMessage
	if decoder.Decode(&fields) != nil || fields == nil {
		return patch, units.ErrEmptyPatch
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return patch, units.ErrEmptyPatch
	}
	for key, value := range fields {
		if strings.TrimSpace(string(value)) == "null" {
			return patch, units.ErrEmptyPatch
		}
		switch key {
		case "name":
			var name string
			if json.Unmarshal(value, &name) != nil {
				return patch, units.ErrInvalidName
			}
			name, err := units.NormalizeName(name)
			if err != nil {
				return patch, err
			}
			patch.Name = &name
		case "active":
			if create {
				return patch, units.ErrEmptyPatch
			}
			var active bool
			if json.Unmarshal(value, &active) != nil {
				return patch, units.ErrEmptyPatch
			}
			patch.Active = &active
		default:
			return patch, units.ErrEmptyPatch
		}
	}
	if create && patch.Name == nil {
		return patch, units.ErrInvalidName
	}
	return patch, patch.Validate()
}

func (s *Server) invalidUnitInput(w http.ResponseWriter) {
	writeError(w, s.log, 400, CodeInvalidRequest, "Parâmetros de unidade inválidos.")
}
func (s *Server) unitError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, units.ErrInvalidName):
		writeError(w, s.log, 400, CodeInvalidRequest, units.ErrInvalidName.Error())
	case errors.Is(err, units.ErrEmptyPatch):
		s.invalidUnitInput(w)
	case errors.Is(err, units.ErrDuplicate):
		writeError(w, s.log, 409, CodeConflict, units.ErrDuplicate.Error())
	case errors.Is(err, units.ErrNotFound):
		writeError(w, s.log, 404, CodeNotFound, units.ErrNotFound.Error())
	default:
		writeError(w, s.log, 503, CodeUnavailable, "Não foi possível acessar as unidades. Tente novamente.")
	}
}
