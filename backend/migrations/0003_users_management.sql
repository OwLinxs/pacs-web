-- Administração V1 — Etapa 2. Forward-only; não altera credenciais/validade.
LOCK TABLE users IN SHARE ROW EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM users WHERE username !~ '^[a-z0-9][a-z0-9_-]{2,63}$') THEN
        RAISE EXCEPTION 'Usernames legados incompativeis; revise antes da migration 0003';
    END IF;
END $$;
ALTER TABLE users DROP CONSTRAINT users_username_format;
ALTER TABLE users ADD CONSTRAINT users_username_format CHECK (username ~ '^[a-z0-9][a-z0-9_-]{2,63}$');
ALTER TABLE users ADD COLUMN must_change_password boolean NOT NULL DEFAULT false;
CREATE TABLE user_units (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    unit_id uuid NOT NULL REFERENCES units(id) ON DELETE RESTRICT,
    PRIMARY KEY (user_id, unit_id)
);
CREATE INDEX user_units_unit_id_idx ON user_units(unit_id);
INSERT INTO user_units(user_id,unit_id)
SELECT id,unit_id FROM users WHERE role <> 'ADMIN' AND unit_id IS NOT NULL;
COMMENT ON COLUMN users.unit_id IS 'Legado preservado; novos vinculos usam user_units. Nao usar para autorizacao.';
COMMENT ON TABLE user_units IS 'Vinculos multiplos; desativar unidade nao remove associacoes.';
