ALTER TABLE criticality_signals
    ADD COLUMN revoked_by uuid REFERENCES accounts(id) ON DELETE RESTRICT;
