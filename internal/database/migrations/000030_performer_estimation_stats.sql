CREATE TABLE actor_estimation_stats (
    actor_id uuid PRIMARY KEY REFERENCES actors(id) ON DELETE CASCADE,
    observation_count integer NOT NULL DEFAULT 0 CHECK (observation_count >= 0),
    mean_relative_error double precision NOT NULL DEFAULT 0,
    relative_error_m2 double precision NOT NULL DEFAULT 0 CHECK (relative_error_m2 >= 0),
    mean_absolute_relative_error double precision NOT NULL DEFAULT 0 CHECK (mean_absolute_relative_error >= 0),
    long_overrun_count integer NOT NULL DEFAULT 0 CHECK (long_overrun_count >= 0),
    long_overrun_severity_sum double precision NOT NULL DEFAULT 0 CHECK (long_overrun_severity_sum >= 0),
    updated_at timestamptz NOT NULL DEFAULT now()
);

COMMENT ON COLUMN actor_estimation_stats.mean_relative_error IS '(actual-promised)/promised; positive values mean slower than promised';
COMMENT ON COLUMN actor_estimation_stats.relative_error_m2 IS 'Welford running M2 for sample variance';
COMMENT ON COLUMN actor_estimation_stats.long_overrun_count IS 'Attempts whose actual duration exceeded twice the promise';
