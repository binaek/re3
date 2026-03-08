# Agent instructions

Guidance for AI agents and tooling working in this repo.

## Git commits

- **Always sign commits.** Never use `--no-gpg-sign` or disable signing.
- When running `git commit` (or any command that creates a commit), use **full permissions** so the process can access the SSH agent socket (`SSH_AUTH_SOCK`). In Cursor, that means `required_permissions: ["all"]` for the commit command; the sandbox otherwise blocks access to the signing agent.
- Commit in reasonable chunks with **imperative, short-but-clear** messages. Do **not** add co-author signatures or other trailers (e.g. `Made-with: Cursor`) to commit messages; omit them entirely.

- **Subject line:** Start with an imperative verb (Add, Fix, Align, Return, …), then the object or change. Put scope at the end if needed (e.g. “in api”, “for api_test”), not as a prefix.  
  - Prefer: **“Add benchmarks for api_test (warm-up, new APIs, …)”**  
  - Avoid: **“api_test: add benchmarks (…)”**
