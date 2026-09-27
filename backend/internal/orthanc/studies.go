package orthanc

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/pmfb-saude/pacs-web/backend/internal/studies"
)

const maxStudyBytes = 4 << 20
const maxSeriesPerStudy = 2000

var resourceID = regexp.MustCompile(`^[a-f0-9]{8}(-[a-f0-9]{8}){4}$`)

type expandedStudy struct {
	ID                   string
	Type                 string
	MainDicomTags        map[string]string
	PatientMainDicomTags map[string]string
	Series               []string
}

// FindStudies realiza somente POST /tools/find (consulta) e GET das séries
// dos estudos da página. O mesmo timeout limita a operação inteira.
func (c *Client) FindStudies(ctx context.Context, cfg Config, query studies.Query) (studies.Page, error) {
	if query.Validate() != nil {
		return studies.Page{}, InvalidTarget
	}
	s, err := c.newSession(cfg)
	if err != nil {
		return studies.Page{}, err
	}
	defer s.client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	filters := map[string]string{}
	for tag, value := range map[string]string{
		"PatientName": query.PatientName, "PatientID": query.PatientID,
		"AccessionNumber": query.AccessionNumber, "StudyDescription": query.StudyDescription,
		"InstitutionName": query.InstitutionName,
	} {
		if value != "" {
			filters[tag] = value
		}
	}
	if query.DateFrom != "" || query.DateTo != "" {
		filters["StudyDate"] = strings.ReplaceAll(query.DateFrom, "-", "") + "-" + strings.ReplaceAll(query.DateTo, "-", "")
	}
	payload, err := json.Marshal(struct {
		Level         string
		Expand        bool
		Limit, Since  int
		CaseSensitive bool
		Query         map[string]string
	}{"Study", true, query.Limit + 1, query.Offset, false, filters})
	if err != nil {
		return studies.Page{}, InvalidTarget
	}
	body, err := s.read(ctx, http.MethodPost, "/tools/find", payload, maxStudyBytes)
	if err != nil {
		return studies.Page{}, err
	}
	var found []expandedStudy
	if json.Unmarshal(body, &found) != nil || found == nil || len(found) > query.Limit+1 {
		return studies.Page{}, InvalidResponse
	}
	seen := map[string]bool{}
	for _, study := range found {
		if study.Type != "Study" || !resourceID.MatchString(study.ID) || seen[study.ID] || study.Series == nil || len(study.Series) > maxSeriesPerStudy {
			return studies.Page{}, InvalidResponse
		}
		seen[study.ID] = true
	}
	page := studies.Page{Items: make([]studies.Study, 0, query.Limit), Limit: query.Limit, Offset: query.Offset, HasMore: len(found) > query.Limit}
	if page.HasMore {
		found = found[:query.Limit]
		if next := query.Offset + query.Limit; next <= studies.MaxOffset {
			page.NextOffset = &next
		}
	}
	for _, study := range found {
		tags, patient := study.MainDicomTags, study.PatientMainDicomTags
		page.Items = append(page.Items, studies.Study{
			OrthancStudyID: study.ID, StudyInstanceUID: tags["StudyInstanceUID"],
			StudyDate: tags["StudyDate"], StudyTime: tags["StudyTime"],
			PatientName: patient["PatientName"], PatientID: patient["PatientID"],
			AccessionNumber: tags["AccessionNumber"], StudyDescription: tags["StudyDescription"],
			InstitutionName: tags["InstitutionName"], Modalities: []string{}, SeriesCount: len(study.Series),
		})
	}
	// No máximo quatro consultas simultâneas por página. Não consulta a sentinela.
	var wg sync.WaitGroup
	var once sync.Once
	var firstErr error
	jobs := make(chan int)
	for worker := 0; worker < min(4, len(found)); worker++ {
		wg.Go(func() {
			for i := range jobs {
				if ctx.Err() != nil {
					continue
				}
				modalities, err := s.studyModalities(ctx, found[i])
				if err != nil {
					once.Do(func() { firstErr = err; cancel() })
					continue
				}
				page.Items[i].Modalities = modalities
			}
		})
	}
	for i := range found {
		select {
		case jobs <- i:
		case <-ctx.Done():
		}
	}
	close(jobs)
	wg.Wait()
	if firstErr != nil {
		return studies.Page{}, firstErr
	}
	if ctx.Err() != nil {
		return studies.Page{}, classify(ctx.Err())
	}
	return page, nil
}

func (s *session) studyModalities(ctx context.Context, study expandedStudy) ([]string, error) {
	modalities := []string{}
	if len(study.Series) == 0 {
		return modalities, nil
	}
	body, err := s.read(ctx, http.MethodGet, "/studies/"+study.ID+"/series?expand=true", nil, maxStudyBytes)
	if err != nil {
		return nil, err
	}
	var series []struct {
		ID, Type, ParentStudy string
		MainDicomTags         map[string]string
	}
	if json.Unmarshal(body, &series) != nil || series == nil || len(series) != len(study.Series) {
		return nil, InvalidResponse
	}
	expected := make(map[string]bool, len(study.Series))
	for _, id := range study.Series {
		expected[id] = true
	}
	unique := map[string]bool{}
	for _, item := range series {
		if item.Type != "Series" || item.ParentStudy != study.ID || !expected[item.ID] {
			return nil, InvalidResponse
		}
		delete(expected, item.ID)
		if modality := strings.TrimSpace(item.MainDicomTags["Modality"]); modality != "" {
			unique[modality] = true
		}
	}
	if len(expected) != 0 {
		return nil, InvalidResponse
	}
	for modality := range unique {
		modalities = append(modalities, modality)
	}
	sort.Strings(modalities)
	return modalities, nil
}
