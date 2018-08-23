### Jira

[INFRA-3183](https://clever.atlassian.net/browse/INFRA-3183): migrate deip repos to CircleCI 2.0

### Overview

**This is an automated PR created with `circle-v2-migrate` v0.11.0.**

CircleCI 1.0 is sunsetting August 31st, meaning CircleCI 1.0 builds will no longer work on September 1st.

This PR uses the output of the `circle-v2-migrate` automigration script, with `microplane`, to translate the build config from circle.yml to CircleCI 2.0's format and file location.

This PR should not make any changes to application code. All test, build, publish, and deploy steps from `circle.yml` should have been preserved in the translation.

If you have any questions, please post in the #circleci-1-sunset channel in slack, and/or join the CircleCI 2.0 migration office hours (see Clever Eng calendar).

### Reviewing & Roll Out Checklist

- [ ] **New: check for redundant steps.**

- [ ] If this repo should be graveyarded, add it to the "graveyard" team in the [CircleCI 1.0 -> 2.0 migration tracking spreadsheet](https://docs.google.com/spreadsheets/d/1Uv6i2TXxZGBUCdjidp2xbqn3gMrgnikJnLgZBXicDBQ/edit?usp=sharing) and close this PR.

- [ ] Debug problems with CircleCI 2.0 build (if needed). Check out go/circleci-ops, #circleci-1-sunset in slack, and/or CircleCI 2.0 office hours (on the Clever Eng calendar) for help.

- [ ] Merge this pull request when the build passes.

- [ ] Deploy as normal for this repository. If the repo's code is not currently deployed (e.g., Dapple says "first deployment"), don't deploy it.

- [ ] Optional: update this repo's row in the [CircleCI 1.0 -> 2.0 migration tracking spreadsheet](https://docs.google.com/spreadsheets/d/1Uv6i2TXxZGBUCdjidp2xbqn3gMrgnikJnLgZBXicDBQ/edit?usp=sharing). (If you don't manually update, the change should show tomorrow.)