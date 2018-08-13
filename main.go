package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Clever/circle-v2-migrate/models"
	// use Clever fork of go-yaml/yaml because go-yaml/yaml limits lines to 80 characters
	"github.com/Clever/yaml"
)

const SCRIPT_VERSION = "0.2.0"

const GOLANG_APP_TYPE = "go"
const NODE_APP_TYPE = "node"
const WAG_APP_TYPE = "wag"
const UNKNOWN_APP_TYPE = "unknown"

const MONGO_DB_TYPE = "mongo"
const POSTGRESQL_DB_TYPE = "postgresql"

// https://circleci.com/docs/2.0/migrating-from-1-2/

// @TODO: add info about target repo (e.g., name) to log lines (kayvee?)
// @TODO: breaks for mongo-to-s3, which uses golang-move-repo ci-scripts script :(
func main() {
	fmt.Printf("circle-v2-migrate v%s\n", SCRIPT_VERSION)
	v1, err := readCircleYaml()
	if err != nil {
		log.Fatal(err)
	}

	v2, err := convertToV2(v1)
	if err != nil {
		log.Fatal(err)
	}

	// fmt.Println("---------- circle YAML Preview ---------")
	marshalled, err := yaml.Marshal(v2)
	if err != nil {
		fmt.Printf("Failed to Marshal v2 yml:\n %s", err)
	} else {
		// fmt.Println(string(marshalled))
	}

	// fmt.Println("----------------------------------------")

	// after translation, write marshalled YAML to .circleci/config.yml
	if _, err := os.Stat("./.circleci"); err != nil {
		os.Mkdir("./.circleci", os.ModePerm)
	}
	outFile, err := os.Create("./.circleci/config.yml")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("writing circleci 2.0 config to .circleci/config.yml")
	_, err = outFile.Write(marshalled)
	if err != nil {
		log.Fatal(err)
	}
	// after translation, remove or rename circle.yml
	fmt.Println("renaming circle.yml -> circle.yml.bak")
	os.Rename("./circle.yml", "./circle.yml.bak")
}

// readCircleYaml reads and parses the repo's circle.yml (V1) file
func readCircleYaml() (models.CircleYamlV1, error) {
	path := "./circle.yml"
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return models.CircleYamlV1{}, fmt.Errorf("circle.yml not found at %s", path)
	}

	contents, err := ioutil.ReadFile(path)
	if err != nil {
		return models.CircleYamlV1{}, err
	}

	var out models.CircleYamlV1
	if err := yaml.Unmarshal(contents, &out); err != nil {
		return models.CircleYamlV1{}, err
	}
	return out, nil
}

// convertToV2 uses the CircleCI 1.0 formatted YAML
// and other cues in the repo (Makefile, presence of swagger.yml, etc)
// to create CircleCI 2.0 formatted YAML
func convertToV2(v1 models.CircleYamlV1) (models.CircleYamlV2, error) {
	v2 := models.CircleYamlV2{
		Version: 2,
	}

	// Determine base image to use based on app type (go/wag/node/...) and language version
	imageConstraints := determineImageConstraints()
	appType := imageConstraints.AppType
	primaryImage := getImage(imageConstraints)
	v2.Jobs.Build.Docker = []models.DockerImage{
		primaryImage,
	}
	// @TODO (INFRA-3159): Determine and add additional database image(s) needed
	dbImages := getDatabaseImages(imageConstraints)
	v2.Jobs.Build.Docker = append(v2.Jobs.Build.Docker, dbImages...)

	// Determine working directory
	workingDir, err := determineWorkingDirectory(appType)
	if err != nil {
		log.Fatal(err)
	}
	v2.Jobs.Build.WorkingDirectory = workingDir

	// Add env vars for directories that were automatically created in CircleCI 1.0
	v2.Jobs.Build.Environment = map[string]string{
		"CIRCLE_ARTIFACTS":    "/tmp/circleci-artifacts",
		"CIRCLE_TEST_REPORTS": "/tmp/circleci-test-results",
	}

	// Clone ci-scripts
	addCloneCIScriptsStep(&v2)

	// Checkout repo
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "checkout")

	// Determine main setup
	for _, item := range v1.Machine.Services {
		if item == "docker" {
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "setup_remote_docker")
		} else {
			fmt.Printf("!WARNING: ingoring v1.Machine.Services item %s\n\n", item)
		}
	}

	// Create directories that were automatically created in CircleCI 1.0
	addCreateCIArtifactDirsStep(&v2)

	// Set up .npmrc if needed (for using private npm packages)
	// @TODO (INFRA-3149): is the set of apps with NODE_APP_TYPE the same as apps with .npmrc_docker files?
	// (this is for when app has .npmrc_docker file)
	if appType == NODE_APP_TYPE {
		addSetupNPMRCStep(&v2)
		addNPMInstallStep(&v2)
	}

	_, usesPostgresql := imageConstraints.DatabaseTypes[POSTGRESQL_DB_TYPE]
	if usesPostgresql {
		addInstallPSQLStep(&v2)
		addWaitForPostgresStep(&v2)
	}

	// translate COMPILE & TEST steps
	translateCompileSteps(&v1, &v2)
	translateTestSteps(&v1, &v2)

	// Install awscli for ECR interactions (used in docker publish deployment steps)
	addInstallAWSCLIStep(&v2)

	// translate and deduplicate DEPLOYMENT steps on master and non-master branches
	err = translateDeploySteps(&v1, &v2)
	if err != nil {
		fmt.Printf("error translating deploy steps: %s\n", err)
		return models.CircleYamlV2{}, err
	}

	return v2, nil
}

func translateCompileSteps(v1 *models.CircleYamlV1, v2 *models.CircleYamlV2) {
	for _, item := range v1.Compile.Pre {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Compile.Override {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Compile.Post {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
}

func translateTestSteps(v1 *models.CircleYamlV1, v2 *models.CircleYamlV2) {
	for _, item := range v1.Test.Pre {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Test.Override {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Test.Post {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
}

func translateDeploySteps(v1 *models.CircleYamlV1, v2 *models.CircleYamlV2) error {
	for key := range v1.Deployment {
		if key != "master" && key != "non-master" {
			return fmt.Errorf("unexpected key in `deployment` map = %s", key)
		}
	}

	nonMaster, nonMasterOk := v1.Deployment["non-master"]
	master, masterOk := v1.Deployment["master"]

	overlap := map[string]interface{}{}
	if masterOk && nonMasterOk {
		for _, mc := range master.Commands {
			for _, nonMc := range nonMaster.Commands {
				if mc == nonMc {
					overlap[mc] = true
				}
			}
		}
	}

	for item := range overlap {
		step := map[string]string{"run": item}
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
	}

	if nonMasterOk {
		for _, item := range nonMaster.Commands {
			if _, isDuplicate := overlap[item]; isDuplicate {
				continue
			}

			step := map[string]string{"run": `if [ "${CIRCLE_BRANCH}" != "master" ]; then ` + item + `; fi;`}
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
		}
	}

	if masterOk {
		for _, item := range master.Commands {
			if _, isDuplicate := overlap[item]; isDuplicate {
				continue
			}

			step := map[string]string{"run": `if [ "${CIRCLE_BRANCH}" == "master" ]; then ` + item + `; fi;`}
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
		}
	}
	return nil
}

func addCreateCIArtifactDirsStep(v2 *models.CircleYamlV2) {
	createCIArtifactsDirsStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "Set up CircleCI artifacts directories",
			"command": `mkdir -p $CIRCLE_ARTIFACTS $CIRCLE_TEST_REPORTS`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, createCIArtifactsDirsStep)
}

func addSetupNPMRCStep(v2 *models.CircleYamlV2) {
	setupNPMRCStep := map[string]interface{}{
		"run": map[string]string{
			"name": "Set up .npmrc",
			"command": `
sed -i.bak s/\${npm_auth_token}/$NPM_TOKEN/ .npmrc_docker
mv .npmrc_docker .npmrc`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, setupNPMRCStep)
}

func addNPMInstallStep(v2 *models.CircleYamlV2) {
	npmInstallStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "npm install",
			"command": "npm install",
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, npmInstallStep)
}

func addInstallAWSCLIStep(v2 *models.CircleYamlV2) {
	installAWSCLIStep := map[string]interface{}{
		"run": map[string]string{
			"name": "Install awscli for ECR publish",
			"command": `cd /tmp/ && wget https://bootstrap.pypa.io/get-pip.py && sudo python get-pip.py
sudo apt-get install python-dev
sudo pip install --upgrade awscli && aws --version
pip install --upgrade --user awscli`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, installAWSCLIStep)
}

func addCloneCIScriptsStep(v2 *models.CircleYamlV2) {
	cloneCIScriptsStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "Clone ci-scripts",
			"command": `cd $HOME && git clone --depth 1 -v https://github.com/Clever/ci-scripts.git && cd ci-scripts && git show --oneline -s`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, cloneCIScriptsStep)
}

func addInstallPSQLStep(v2 *models.CircleYamlV2) {
	installPSQLStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "Install psql",
			"command": "sudo apt-get install postgresql",
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, installPSQLStep)
}

func addWaitForPostgresStep(v2 *models.CircleYamlV2) {
	waitForPostgresStep := map[string]interface{}{
		"run": map[string]string{
			"name": "Wait for postgres database to be ready",
			"command": `echo Waiting for postgres
for i in ` + "`seq 1 10`;" + `
do
  nc -z localhost 5432 && echo Success && exit 0
  echo -n .
  sleep 1
done
echo Failed waiting for postgres && exit 1`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, waitForPostgresStep)

}

func determineWorkingDirectory(appType string) (string, error) {
	// @TODO: determine decent working directory depending on app type for non-(go, wag, node) apps
	// go, wag: /go/src/github.com/Clever/catapult
	// non-wag node: ~/Clever/hubble

	// get repo
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	splitDir := strings.Split(dir, "/")
	if len(splitDir) < 1 {
		return "", fmt.Errorf("failed to find repo in %s", dir)
	}
	repo := splitDir[len(splitDir)-1]
	// for microplane compaibility:
	if repo == "planned" {
		repo = splitDir[len(splitDir)-3]
	}

	// put together working directory string
	if appType == GOLANG_APP_TYPE || appType == WAG_APP_TYPE {
		return fmt.Sprintf("/go/src/github.com/Clever/%s", repo), nil
	}
	return fmt.Sprintf("~/Clever/%s", repo), nil
}

// determineImageConstraints returns the constraints for the docker images section of build, including:
// -- app type (wag, go, node, unknown)
// -- version of  image base language/library (e.g., go "1.10", node "6")
// -- database types needed for tests (e.g., mongo, postgresql)
func determineImageConstraints() models.ImageConstraints {
	// if node, will have package.json and node.mk (but this is clever-specific) in main project dir
	// if go, will have golang.mk (but this is clever-specific)
	// another common occurance is go with node, which for us is mostly wag
	imageConstraints := models.ImageConstraints{
		AppType: "unknown",
	}
	if _, err := os.Stat("./swagger.yml"); err == nil {
		imageConstraints = models.ImageConstraints{
			AppType: WAG_APP_TYPE,
			Version: determineGoVersion(),
		}
	} else if _, err := os.Stat("./golang.mk"); err == nil {
		imageConstraints = models.ImageConstraints{
			AppType: GOLANG_APP_TYPE,
			Version: determineGoVersion(),
		}
	} else if _, err := os.Stat("./node.mk"); err == nil {
		imageConstraints = models.ImageConstraints{
			AppType: NODE_APP_TYPE,
			Version: determineNodeVersion(),
		}
	}
	imageConstraints.DatabaseTypes = determineDatabaseTypes()
	return imageConstraints
}

// determineDatabaseTypes returns a set of database types needed for tests
func determineDatabaseTypes() map[string]struct{} {
	databaseTypes := map[string]struct{}{}
	if needsPostgreSQL() {
		databaseTypes[POSTGRESQL_DB_TYPE] = struct{}{}
	}
	if needsMongoDB() {
		databaseTypes[MONGO_DB_TYPE] = struct{}{}
	}
	return databaseTypes
}

// needsPostgreSQL returns true if tests rely on postgresql, based on these criteria:
// -- true if a Makefile contains the text `psql`
// -- false otherwise
func needsPostgreSQL() bool {
	// @TODO: could also check for pq in Gopkg.toml, for go repos -- but does this always mean it's used in tests?
	postgresqlCheckRegexp := regexp.MustCompile(`psql`)
	makefile, err := ioutil.ReadFile("Makefile")
	if err != nil {
		log.Fatal(err)
	}
	return postgresqlCheckRegexp.Match(makefile)
}

// needsMongoDB returns true if tests rely on mongodb, based on these criteria:
// -- true if Makefile contains the text `MONGO_TEST_DB`
// -- true if a file with `test` in the name contains the text `testMongoURL`
// -- false otherwise
func needsMongoDB() bool {
	// @TODO: could also check for mgo in Gopkg.toml, for go repos -- but does this always mean it's used in tests?
	// check Makefile for MONGO_TEST_DB
	// @TODO update comment; use "or" in regexp
	mongoCheckRegexp := regexp.MustCompile(`MONGO_TEST_DB|mongodb://localhost|mongodb://127.0.0.1`)
	makefile, err := ioutil.ReadFile("Makefile")
	if err != nil {
		log.Fatal(err)
	}
	if mongoCheckRegexp.Match(makefile) {
		return true
	}
	// check test files for testMongoURL
	// grep --include=\*test* -rnw . -e "testMongoURL" --exclude-dir={vendor,gen-*}
	cmd := exec.Command("/bin/sh", "-c", "fgrep --include=\\*test* -rnw . -e \"testMongoURL\" --exclude-dir={vendor,gen-*}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(string(output)) > 0 {
			fmt.Printf("\n\n!Warning: failed to check for mongo. Error: %s\n\n", string(output))
		}
		return false
	}
	return len(string(output)) > 0
}

// determineGoVersion determines version of go in use for an app
// this information is in makefile's golang-version-check, e.g.:
// $(eval $(call golang-version-check,1.10))
// @TODO: error if version not found instead of returning 1.10? or is 1.10 default alright?
func determineGoVersion() string {
	version := "1.10"
	versionCheckRegexp := regexp.MustCompile(`golang-version-check,([0-1].[0-9]+)`)
	makefile, err := ioutil.ReadFile("Makefile")
	if err != nil {
		log.Fatal(err)
	}
	versionCheck := versionCheckRegexp.FindSubmatch(makefile)
	if versionCheck != nil {
		version = string(versionCheck[1])
	}
	return version
}

// determineNodeVersion determines version of node for an app
// @TODO (INFRA-3156): implement (determine correct node version for non-wag node apps)
func determineNodeVersion() string {
	version := "8"
	versionCheckRegexp := regexp.MustCompile(`NODE_VERSION := "v([0-9]+)"`)
	makefile, err := ioutil.ReadFile("Makefile")
	if err != nil {
		log.Fatal(err)
	}
	versionCheck := versionCheckRegexp.FindSubmatch(makefile)
	if versionCheck != nil {
		version = string(versionCheck[1])
	}
	return version
}

// getImage returns the primary image needed for a repo to build, based on app type and version
func getImage(constraints models.ImageConstraints) models.DockerImage {
	// @TODO (INFRA-3163): add human-readable image tags/other comments for image, if doable in yaml
	// @TODO: use SHAs for all images
	appType := constraints.AppType
	version := constraints.Version
	// default image (reproduces CircleCI 1.0 base)
	defaultImage := models.DockerImage{
		Image: "circleci/build-image:ubuntu-14.04-XXL-upstart-1189-5614f37",
	}

	// @TODO (INFRA-3149): node version for wag locked in at 8.11.3 by these images -- could be ok (?)
	golangImageMap := map[string]models.DockerImage{
		"1.10": models.DockerImage{Image: "circleci/golang:1.10.3-stretch"}, // "circleci/golang@sha256:4614481a383e55eef504f26f383db1329c285099fde0cfd342c49e5bb9b6c32a"
		"1.9":  models.DockerImage{Image: "circleci/golang:1.9.7-stretch"},  // "circleci/golang@sha256:c46bee0b60747525d354f219083a46e06c68152f90f3bfb2812d1f232e6a5097"
		"1.8":  models.DockerImage{Image: "circleci/golang:1.8.7-stretch"},
	}

	// @TODO (INFRA-3149): base image for node <6 that's not just the xxl default?
	nodeImageMap := map[string]models.DockerImage{
		"10": models.DockerImage{Image: "circleci/node:10.8.0-stretch"},
		"8":  models.DockerImage{Image: "circleci/node:8.11.3-stretch"},
		"6":  models.DockerImage{Image: "circleci/node:6.14.3-stretch"},
	}

	if appType == GOLANG_APP_TYPE || appType == WAG_APP_TYPE {
		golangBaseImage, ok := golangImageMap[version]
		if ok {
			return golangBaseImage
		}
	} else if appType == NODE_APP_TYPE {
		nodeBaseImage, ok := nodeImageMap[version]
		if ok {
			return nodeBaseImage
		}
	}
	fmt.Printf("No circleci image selected for app type %s, version %s -- using default\n", constraints.AppType, constraints.Version)
	return defaultImage
}

// getDatabaseImages returns a slice of database images that a repo needs to build
// (over and above its primary, base image) based on database types it uses
func getDatabaseImages(constraints models.ImageConstraints) []models.DockerImage {
	dbImageMap := map[string]models.DockerImage{
		// @TODO: SHAs, also decide most appropriate images to use
		POSTGRESQL_DB_TYPE: models.DockerImage{
			Image: "circleci/postgres:9.4-alpine-ram",
		},
		MONGO_DB_TYPE: models.DockerImage{
			// @TODO: 3.4?
			Image: "circleci/mongo:3.2.20-jessie-ram",
		},
	}
	dbImages := []models.DockerImage{}
	var dbImage models.DockerImage
	var ok bool
	for dbType, _ := range constraints.DatabaseTypes {
		dbImage, ok = dbImageMap[dbType]
		if !ok {
			fmt.Printf("Error!!! -- cannot find database image for database type %s\n", dbType)
		}
		dbImages = append(dbImages, dbImage)
	}
	return dbImages
}
