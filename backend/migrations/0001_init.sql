-- 0001_init — fundação do PACS Web Municipal.
--
-- Este banco pertence exclusivamente à nova aplicação. O PostgreSQL usado
-- internamente pelo Orthanc é outro sistema e não é tocado por estas migrations.

CREATE TABLE units (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        text        NOT NULL UNIQUE,
    name        text        NOT NULL,
    kind        text        NOT NULL,
    active      boolean     NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT units_slug_format CHECK (slug ~ '^[a-z0-9][a-z0-9-]{1,48}$'),
    CONSTRAINT units_kind_valid  CHECK (kind IN ('UPA', 'UNIDADE_18H', 'UBS', 'HOSPITAL', 'OUTRO'))
);

COMMENT ON TABLE units IS 'Unidades de saúde. Administráveis pela interface numa etapa futura; nada é fixado em código.';

CREATE TABLE users (
    id                 uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text        NOT NULL,
    username           text        NOT NULL UNIQUE,
    email              text,
    password_hash      text        NOT NULL,
    role               text        NOT NULL,
    unit_id            uuid        REFERENCES units (id) ON DELETE SET NULL,
    active             boolean     NOT NULL DEFAULT true,
    access_valid_until date,
    last_login_at      timestamptz,
    created_at         timestamptz NOT NULL DEFAULT now(),
    updated_at         timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_role_valid      CHECK (role IN ('ADMIN', 'GESTOR', 'MEDICO')),
    CONSTRAINT users_username_format CHECK (username ~ '^[a-z0-9][a-z0-9._-]{2,63}$'),
    CONSTRAINT users_name_present    CHECK (length(btrim(name)) > 0)
);

COMMENT ON COLUMN users.username IS 'Sempre normalizado em minúsculas pela aplicação; a unicidade é garantida aqui.';
COMMENT ON COLUMN users.password_hash IS 'Argon2id no formato PHC. Nunca sai pela API nem entra em log.';
COMMENT ON COLUMN users.access_valid_until IS 'Último dia de acesso permitido, inclusive. NULL = sem prazo.';

CREATE INDEX users_unit_id_idx ON users (unit_id);
CREATE INDEX users_role_idx    ON users (role);

CREATE TABLE sessions (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    token_hash          bytea       NOT NULL UNIQUE,
    user_id             uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at          timestamptz NOT NULL DEFAULT now(),
    last_seen_at        timestamptz NOT NULL DEFAULT now(),
    absolute_expires_at timestamptz NOT NULL,
    revoked_at          timestamptz,
    ip_address          inet,
    user_agent          text
);

COMMENT ON TABLE sessions IS 'Sessões server-side. Guarda apenas o SHA-256 do identificador; o token bruto existe somente no cookie.';

CREATE INDEX sessions_user_id_idx    ON sessions (user_id);
CREATE INDEX sessions_expires_at_idx ON sessions (absolute_expires_at);

CREATE TABLE audit_events (
    id              bigserial   PRIMARY KEY,
    occurred_at     timestamptz NOT NULL DEFAULT now(),
    event           text        NOT NULL,
    actor_user_id   uuid        REFERENCES users (id) ON DELETE SET NULL,
    actor_username  text,
    unit_id         uuid        REFERENCES units (id) ON DELETE SET NULL,
    detail          text,
    origin          text
);

COMMENT ON TABLE audit_events IS 'Trilha de auditoria. Nunca recebe senha, hash, token, cookie, header de autorização ou dado identificável de paciente.';
COMMENT ON COLUMN audit_events.actor_username IS 'Cópia do username tentado, necessária em LOGIN_FAILURE quando não existe usuário.';

CREATE INDEX audit_events_occurred_at_idx ON audit_events (occurred_at DESC);
CREATE INDEX audit_events_event_idx       ON audit_events (event);
CREATE INDEX audit_events_actor_idx       ON audit_events (actor_user_id);

CREATE TABLE app_settings (
    key        text        PRIMARY KEY,
    value      jsonb       NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now(),
    updated_by uuid        REFERENCES users (id) ON DELETE SET NULL
);

COMMENT ON TABLE app_settings IS 'Configuração operacional administrável. Valores sensíveis serão gravados cifrados com a chave mestra externa (etapa futura).';
