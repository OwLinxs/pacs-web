-- Administração V1 — Etapa 1: Unidades.
-- Preserva a tabela existente, seus IDs, slug/kind e referências de usuários.
-- Nenhuma unidade é criada, renomeada, excluída ou desativada automaticamente.
-- A migration é executada na transação do runner existente.
LOCK TABLE units IN SHARE ROW EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM units WHERE char_length(btrim(name)) NOT BETWEEN 1 AND 120
               OR name ~ '^[[:space:]]*$' OR name ~ '[[:cntrl:]]') THEN
        RAISE EXCEPTION 'Unidades legadas possuem nomes invalidos; revise os registros antes da migration 0002';
    END IF;
    IF EXISTS (SELECT 1 FROM units GROUP BY lower(btrim(name)) HAVING count(*) > 1) THEN
        RAISE EXCEPTION 'Unidades legadas possuem nomes duplicados; revise os registros antes da migration 0002';
    END IF;
END $$;

ALTER TABLE units ADD CONSTRAINT units_name_valid CHECK (
    char_length(btrim(name)) BETWEEN 1 AND 120
    AND name !~ '^[[:space:]]*$'
    AND name !~ '[[:cntrl:]]'
);
CREATE UNIQUE INDEX units_name_ci_unique ON units (lower(btrim(name)));
COMMENT ON TABLE units IS 'Entidades próprias; somente ADMIN administra. Desativação lógica, sem exclusão. Associação muitos-para-muitos com usuários fica para etapa futura.';
