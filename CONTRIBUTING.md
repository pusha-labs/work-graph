# Contributing to Work Graph

Work Graph is still an early MVP. Contributions should keep the product understandable, preserve its domain rules, and avoid introducing provider-specific concepts into the core.

## Start locally

The supported development path uses Docker Compose:

```sh
docker compose up --build
```

Open `http://localhost:8088`. To explore a populated workspace, follow the demo-data instructions in the README.

## Before changing code

- Read `docs/vision.md` and `docs/concepts.md` for product language.
- Check `docs/adr/` for accepted architectural decisions.
- Keep Jira and other provider identity inside the future Integration Gateway boundary described by ADR 0005.
- Open an issue or discussion before a large domain, security, persistence, or UI-direction change.

## Verification

Run the same checks used by CI:

```sh
docker build --target test .
docker build ./apps/web
bash -n scripts/*.sh
./scripts/test-backup-restore.sh
./scripts/test-demo.sh
```

The backup/restore and demo tests create isolated Compose projects and volumes and remove them after the test. The demo test also asks Docker for a random host port, so neither test uses or interrupts the ordinary development installation.

## Change guidelines

- Preserve workspace authorization and attribute accepted changes to an actor.
- Treat history as product data, not disposable logging.
- Use public domain commands instead of writing around invariants.
- Keep workflow module execution outside the API process.
- Explain diagnostics using supporting facts and avoid presenting organizational risk as a negative judgment about a person.
- Keep the primary tree interface minimal; reveal secondary controls contextually.
- Add or update tests for behavior changes.
- Update the relevant concept, ADR, roadmap, or operating guide when a decision changes.

## Pull requests

Keep pull requests focused. Describe:

- the user problem;
- the chosen behavior;
- important tradeoffs or security implications;
- how the change was verified;
- any migration, deployment, or rollback considerations.

CI must pass before merge. Do not include passwords, session tokens, runner tokens, encryption keys, populated `.env` files, database archives, or private work data.

## License

By contributing, you agree that your contribution is licensed under the repository's [GNU Affero General Public License v3.0 only](LICENSE).
