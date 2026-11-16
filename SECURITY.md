# Security

## Reporting a vulnerability

Please report security problems privately, not in a public issue. On this repository's **Security** tab, choose **Report a vulnerability**: it reaches the maintainer alone.

Say what you found, how to reproduce it, and what someone could do with it. You can expect an answer within a week. A fix comes in a release before the details are published, and you are credited in its notes unless you would rather not be.

## Supported versions

Only the latest release gets security fixes. Upgrading is described in [docs/self-hosting.md](docs/self-hosting.md#upgrading).

## Checking a release

Every file in a release has a checksum in `SHA256SUMS` and GitHub's attestation, in `mila.intoto.jsonl`, that Mila's release workflow built it from the release's tag. [docs/self-hosting.md](docs/self-hosting.md#option-3-the-program-on-its-own) shows how to check one.

## What Mila does to stay safe

How sign-in, sessions, permissions and the rest are built, and what each security review found and changed, is in [docs/PLAN.md](docs/PLAN.md), section 9.
