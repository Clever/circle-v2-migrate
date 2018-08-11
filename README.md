# circle-v2-migrate

Migrates a repo from CircleCI v1 to v2

owned by eng-infra

## Why migrate

CircleCI 1.0 is sunsetting August 31st, 2018, meaning CircleCI 1.0 builds will no longer work on September 1st.

We must migrate to CircleCI 2.0 before then to maintain our current test/build/deploy flows. 

CircleCI 2.0 also promises faster builds. 

## Features

- directly translates compile and test steps from CircleCI 1.0 config to 2.0 format
- translates and dedupes deploy steps
- for node>=6 and go>=1.8, determines which smaller base image to use instead of CircleCI 1.0's giant base image (faster than v1!) 
- for mongodb and postgresql, detects when we use database in tests and adds a database image to the CircleCI 2.0 config (faster than v1!)


## Questions or Concerns?

Post in #circleci-1-sunset in slack! If your question or concern is about a particular repo, please also add a note or failing build link for the repo in [CircleCI 1.0 -> 2.0 migration tracking spreadsheet](https://docs.google.com/spreadsheets/d/1Uv6i2TXxZGBUCdjidp2xbqn3gMrgnikJnLgZBXicDBQ/edit?usp=sharing).

