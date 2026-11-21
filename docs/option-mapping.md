# Option mapping

How each `Config` field reaches Ansible: as a CLI flag, as an environment variable, through a temp file, or not at
all. For library users who wonder why an option has no effect, and for contributors adding a field.

The per-field source of truth is the doc comment in `ansiblePlaybook.go`; this page groups the fields by path.

## CLI flags

Most fields map 1:1 to a flag of the same name. The doc comment says "maps to ansible-playbook --x" or
"maps to ansible-galaxy --x". Zero values (`""`, `0`, `false`, empty slice) add no flag.

| Rule                                 | Fields                                                                                                                          |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| One flag per slice entry             | `Inventories` (`--inventory`), `ExtraVars` (`--extra-vars`), `ModulePath` (`--module-path`), `GalaxyIgnoreSignatureStatusCodes` |
| Positional arguments                 | `Playbooks` (paths, globs or collection FQCN `namespace.collection.name`)                                                       |
| Rendered as `-v` … `-vvvv`           | `Verbose` (capped at 4, applied to galaxy and playbook)                                                                         |
| Galaxy only, run before the playbook | all `Galaxy*` fields except `GalaxyAPIKey`; triggered by `GalaxyFile` or `GalaxyRequirementsFile`                               |

## Environment variables

`ansible-playbook` has no flag for these, so they are set on the command environment.

| Field                | Variable                                                      |
| -------------------- | ------------------------------------------------------------- |
| `ConfigFile`         | `ANSIBLE_CONFIG` (file must exist)                            |
| `FactPath`           | `ANSIBLE_FACT_PATH`                                           |
| `FactCaching`        | `ANSIBLE_FACT_CACHING`                                        |
| `FactCachingTimeout` | `ANSIBLE_FACT_CACHING_TIMEOUT` (only when > 0)                |
| `OutputCallback`     | `ANSIBLE_STDOUT_CALLBACK`                                     |
| `StrategyPlugin`     | `ANSIBLE_STRATEGY`                                            |
| `CallbacksEnabled`   | `ANSIBLE_CALLBACKS_ENABLED`                                   |
| `PollInterval`       | `ANSIBLE_POLL_INTERVAL`                                       |
| `AnyErrorsFatal`     | `ANSIBLE_ANY_ERRORS_FATAL=True`                               |
| `SSHTransferMethod`  | `ANSIBLE_SSH_TRANSFER_METHOD`                                 |
| `GalaxyAPIKey`       | `ANSIBLE_GALAXY_TOKEN`                                        |
| `NoColor`            | `ANSIBLE_NOCOLOR=1`, drops an inherited `ANSIBLE_FORCE_COLOR` |

Order in the environment, later wins:

1. Inherited process environment
2. `ANSIBLE_GALAXY_DISPLAY_PROGRESS=0`, then color: `ANSIBLE_NOCOLOR=1` with `NoColor`, otherwise
   `ANSIBLE_FORCE_COLOR=1` only when `Stdout` is an `*os.File`
3. `ANSIBLE_GALAXY_TOKEN`
4. The variables from the table above
5. `ExtraEnv`, sorted by key, so it can override everything before it

## Temp files

| Field           | Effect                                                                                                           |
| --------------- | ---------------------------------------------------------------------------------------------------------------- |
| `PrivateKey`    | Written to `TempDir` (mode `0600`), passed as `--private-key`; takes precedence over `PrivateKeyFile`            |
| `VaultPassword` | Written to `TempDir` (mode `0600`), passed as `--vault-password-file`; takes precedence over `VaultPasswordFile` |

The files are removed after `Exec`. Set `TempDir` to a tmpfs mount if secrets must not reach persistent disk.

## Rejected

`GatherSubset`, `GatherTimeout` and `MaxFailPercentage` are play keywords only. `Exec` returns an error when one of
them is set; set them on the play instead.
