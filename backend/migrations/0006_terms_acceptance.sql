-- Aceite do termo de responsabilidade e sigilo, por usuário e versão do texto.
-- Uma nova versão do termo exige novo aceite; o histórico das anteriores fica.
CREATE TABLE terms_acceptances (
    user_id     uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    version     text        NOT NULL CHECK (version ~ '^[0-9a-z.-]{1,40}$'),
    accepted_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, version)
);

COMMENT ON TABLE terms_acceptances IS 'Ciência do termo de responsabilidade e sigilo. Não é base legal do tratamento (LGPD art. 11, II); comprova a orientação do profissional.';
