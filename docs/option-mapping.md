# Option mapping

How each `Config` field reaches Ansible: as a CLI flag, as an environment variable, through a temp file, or not at
all. For library users who wonder why an option has no effect, and for contributors adding a field.

The per-field source of truth is the doc comment in `ansiblePlaybook.go`; this page groups the fields by path.

## CLI flags

Most fields map to one flag; the doc comment names it: "maps to ansible-playbook --x" or
"maps to ansible-galaxy --x". Zero values (`""`, `0`, `false`, empty slice) add no flag.

| Rule                                 | Fields                                                                                                                                                                                                                            |
| ------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| One flag per non-empty slice entry   | `Inventories` (`--inventory`), `ExtraVars` (`--extra-vars`), `ModulePath` (`--module-path`), `GalaxyIgnoreSignatureStatusCodes` (`--ignore-signature-status-code`)                                                                |
| Positional arguments                 | `Playbooks` (paths, globs or collection FQCN `namespace.collection.name`)                                                                                                                                                         |
| Rendered as `-v` … `-vvvv`           | `Verbose` (capped at 4, applied to galaxy and playbook)                                                                                                                                                                           |
| Galaxy only, run before the playbook | all `Galaxy*` fields except `GalaxyAPIKey`; triggered by `GalaxyFile` or `GalaxyRequirementsFile` (file must exist), whose value becomes `--role-file` and `--requirements-file`; `GalaxyRequirementsFile` overrides `GalaxyFile` |
| Separate command                     | `ShowVersion` runs `ansible --version` before everything else                                                                                                                                                                     |

## Environment variables

`ansible-playbook` has no flag for these, so they are set on the command environment.

| Field                | Variable                                                                   |
| -------------------- | -------------------------------------------------------------------------- |
| `ConfigFile`         | `ANSIBLE_CONFIG` (`Exec` and `CommandStrings` fail if the file is missing) |
| `FactPath`           | `ANSIBLE_FACT_PATH`                                                        |
| `FactCaching`        | `ANSIBLE_FACT_CACHING`                                                     |
| `FactCachingTimeout` | `ANSIBLE_FACT_CACHING_TIMEOUT` (only when > 0)                             |
| `OutputCallback`     | `ANSIBLE_STDOUT_CALLBACK`                                                  |
| `StrategyPlugin`     | `ANSIBLE_STRATEGY`                                                         |
| `CallbacksEnabled`   | `ANSIBLE_CALLBACKS_ENABLED`                                                |
| `PollInterval`       | `ANSIBLE_POLL_INTERVAL` (only when > 0)                                    |
| `AnyErrorsFatal`     | `ANSIBLE_ANY_ERRORS_FATAL=True`                                            |
| `SSHTransferMethod`  | `ANSIBLE_SSH_TRANSFER_METHOD`                                              |
| `GalaxyAPIKey`       | `ANSIBLE_GALAXY_TOKEN`                                                     |
| `NoColor`            | `ANSIBLE_NOCOLOR=1`, drops an inherited `ANSIBLE_FORCE_COLOR`              |

Order in the environment, later wins:

1. Inherited process environment
2. `ANSIBLE_GALAXY_DISPLAY_PROGRESS=0`, then color: `ANSIBLE_NOCOLOR=1` with `NoColor`, otherwise
   `ANSIBLE_FORCE_COLOR=1` only when `Stdout` is an `*os.File` and neither `NO_COLOR` nor
   `ANSIBLE_NOCOLOR` is true
3. `ANSIBLE_GALAXY_TOKEN`
4. The remaining variables from the table above, from `ConfigFile` to `SSHTransferMethod`
5. `ExtraEnv`, sorted by key, so it can override everything before it

`trace` and `CommandStrings` show steps 3 and 4 as a `KEY=VALUE` prefix before each command, with
`ANSIBLE_GALAXY_TOKEN` masked. The inherited environment, the color and progress variables and `ExtraEnv` are not shown.

## Temp files

| Field           | Effect                                                                                                                                                                                                        |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `PrivateKey`    | Written to `TempDir` (mode `0600`), passed as `--private-key`; takes precedence over `PrivateKeyFile`. Must be PEM with `BEGIN`/`END … PRIVATE KEY` markers, otherwise `Exec` fails; CRLF is normalized to LF |
| `VaultPassword` | Written to `TempDir` (mode `0600`), passed as `--vault-password-file`; takes precedence over `VaultPasswordFile`                                                                                              |

The files are removed after `Exec`. Set `TempDir` to a tmpfs mount if secrets must not reach persistent disk.

## Rejected

`GatherSubset`, `GatherTimeout` and `MaxFailPercentage` are play keywords only. `Exec` and `CommandStrings` return an
error when one of them is set; set them on the play instead.
