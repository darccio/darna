<!--
title: darna
summary: Validate atomic Go commits before saving a changeset.
language: en
feed: false
-->
# darna

Darna checks whether staged Go files form an atomic commit: every referenced symbol must be defined in the staged changeset or in already-committed code. It reports dependencies on unstaged or untracked files before they break a commit.

## Install

```sh
go install dario.cat/darna/cmd/darna@latest
```

Darna requires Go 1.24 or newer and `git` on `PATH`.

## Validate a commit

Stage the files that belong together, then run:

```sh
darna
```

An exit code of zero means the staged changes are atomic. An exit code of one reports missing dependencies.

To find the next independent file and its direct dependants before staging, run:

```sh
darna --committable --dependants
```

For partial staging, Darna reads the indexed file contents so the analysis reflects the changes that will be committed.

## More information

- [Full CLI documentation](https://github.com/darccio/darna#usage)
- [Source repository](https://github.com/darccio/darna)
- [Architecture decisions](https://github.com/darccio/darna/tree/main/docs/decisions)
- [Report a bug](https://github.com/darccio/darna/issues)
- [Sponsor maintenance](https://github.com/sponsors/darccio)

## License

Darna and these docs are distributed under the [Apache License 2.0](LICENSE.txt).
