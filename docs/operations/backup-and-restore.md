# Backup and Restore

Work Graph stores its durable product state in PostgreSQL. The repository provides scripts that create and verify portable backup archives while keeping encryption keys outside those archives.

## Create a backup

With the Compose installation running:

```sh
./scripts/backup.sh
```

The archive is written to `backups/work-graph-<UTC timestamp>.tar.gz` with owner-only permissions. A different destination can be supplied explicitly:

```sh
./scripts/backup.sh /path/to/safe-storage/work-graph.tar.gz
```

Each archive contains only:

- `database.dump`, a PostgreSQL custom-format dump;
- `manifest.txt`, containing the archive format, creation time, application revision, schema version, and dump checksum.

The script does not copy `.env`, provider credentials, or `WORK_GRAPH_MASTER_KEY`.

## Protect the encryption key

Back up `WORK_GRAPH_MASTER_KEY` separately from the database archive. Store it in a password manager or another protected secrets system.

Database restoration does not require the key, but encrypted workspace secrets restored from the database cannot be read or used without the exact key that encrypted them. Losing the key is intentionally unrecoverable.

## Restore a backup

Restoration replaces the current database. Verify the archive path, then provide the explicit confirmation value:

```sh
RESTORE_CONFIRM=replace-data ./scripts/restore.sh /path/to/work-graph.tar.gz
```

Before replacing data, the restore script:

1. rejects archives containing unexpected paths;
2. verifies the archive format and SHA-256 checksum;
3. refuses a database schema newer than the checked-out application supports;
4. creates a timestamped `pre-restore` safety backup of the current database;
5. stops API and runner processes so no writes occur during replacement.

After a successful import, the API starts and applies any migrations newer than the restored backup. The script prints the location of the safety backup.

If import fails, application services remain stopped and the safety archive is preserved. Fix the cause or restore the safety archive explicitly; do not repeatedly run migrations against a partially restored database.

For disaster recovery into a genuinely empty installation, or when the current database is too damaged to dump, the safety step can be bypassed only with the explicit `SKIP_SAFETY_BACKUP=empty-or-unrecoverable-database` value. Use this exception only after confirming that no recoverable current data exists:

```sh
RESTORE_CONFIRM=replace-data \
SKIP_SAFETY_BACKUP=empty-or-unrecoverable-database \
./scripts/restore.sh /path/to/work-graph.tar.gz
```

## Verify the procedure

The destructive verification test uses an isolated Compose project and volume. It does not touch the ordinary `work-graph` database:

```sh
make test-backup
```

The test creates a marker, backs it up, changes it, restores the archive, verifies the original value, and then removes its isolated containers and volume.

## Operational recommendations

- Copy archives away from the Docker host; a backup on the same disk is not sufficient protection.
- Keep multiple dated archives and periodically run the isolated verification test.
- Protect archives as confidential data: they contain work, people, history, and encrypted secret records.
- Record which `WORK_GRAPH_MASTER_KEY` belongs to each installation without placing the key inside the archive.
- Create an on-demand backup before upgrading Work Graph.
