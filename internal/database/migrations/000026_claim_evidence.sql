ALTER TABLE actor_capabilities
    ADD COLUMN claim_level text NOT NULL DEFAULT 'working' CHECK (claim_level IN ('awareness','working','advanced','expert')),
    ADD COLUMN evidence_note text NOT NULL DEFAULT '' CHECK (length(evidence_note) <= 1000),
    ADD COLUMN reviewed_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE actor_knowledge
    ADD COLUMN claim_level text NOT NULL DEFAULT 'working' CHECK (claim_level IN ('awareness','working','advanced','expert')),
    ADD COLUMN evidence_note text NOT NULL DEFAULT '' CHECK (length(evidence_note) <= 1000),
    ADD COLUMN reviewed_at timestamptz NOT NULL DEFAULT now();
