-- Estatísticas V2: somente contagens agregadas, sem identificador de estudo,
-- paciente ou usuário. Recalculáveis a partir do Orthanc a qualquer momento.
CREATE TABLE stats_daily (
    day          date        PRIMARY KEY,
    exams        integer     NOT NULL CHECK (exams >= 0),
    without_time integer     NOT NULL CHECK (without_time >= 0 AND without_time <= exams),
    by_hour      integer[]   NOT NULL CHECK (cardinality(by_hour) = 24),
    truncated    boolean     NOT NULL DEFAULT false,
    computed_at  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE stats_daily IS 'Exames por dia civil de São Paulo (StudyDate), com distribuição por hora (StudyTime). Recalculada pelo job de estatísticas.';

-- Retrato diário do acervo (GET /statistics do Orthanc). Não há como reconstruir
-- dias anteriores à implantação: o histórico começa quando o job passa a rodar.
CREATE TABLE stats_archive_daily (
    day         date        PRIMARY KEY,
    studies     bigint      NOT NULL CHECK (studies >= 0),
    series      bigint      NOT NULL CHECK (series >= 0),
    instances   bigint      NOT NULL CHECK (instances >= 0),
    disk_mb     bigint      NOT NULL CHECK (disk_mb >= 0),
    captured_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE stats_archive_daily IS 'Último retrato do acervo do Orthanc em cada dia civil de São Paulo.';
