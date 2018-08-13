### Jira

[INFRA-3174](https://clever.atlassian.net/browse/INFRA-3174): migrate initial repos to CircleCI 2.0

### Overview

**This is an automated PR created with `circle-v2-migrate` v0.2.0.**

CircleCI 1.0 is sunsetting August 31st, meaning CircleCI 1.0 builds will no longer work on September 1st.

This PR uses the output of the `circle-v2-migrate` automigration script, with `microplane`, to translate the build config from circle.yml to CircleCI 2.0's format and file location.

This PR should not make any changes to application code.

### Reviewing

**Please check the following:**

- [ ] build works

If the build fails, please link to the failing build in this repo's row in the [CircleCI 1.0 -> 2.0 migration tracking spreadsheet](https://docs.google.com/spreadsheets/d/1Uv6i2TXxZGBUCdjidp2xbqn3gMrgnikJnLgZBXicDBQ/edit?usp=sharing). In the week of August 13, infra is still improving the automigration script based on this feedback.

If you are trying to debug a failed build, or if you have any other questions or concerns, please post in the #circleci-1-sunset channel in slack. You're also welcome to join the CircleCI 2.0 office hours listed in the Clever Eng calendar.


### Roll Out

**Please merge this pull request when you are ready.**

All test, build, publish, and deploy steps should have been preserved in the translation.

Use normal rollout practices for this repository.