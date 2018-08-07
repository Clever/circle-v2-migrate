package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/Clever/circle-v2-migrate/models"
	yaml "gopkg.in/yaml.v2"
)

const GOLANG_APP_TYPE = "go"
const NODE_APP_TYPE = "node"
const WAG_APP_TYPE = "wag"
const UNKNOWN_APP_TYPE = "unknown"

// https://circleci.com/docs/2.0/migrating-from-1-2/

// @TODO: add info about target repo (e.g., name) to log lines (kayvee?)
func main() {
	v1, err := readCircleYaml()
	if err != nil {
		log.Fatal(err)
	}

	v2, err := convertToV2(v1)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("V1")
	fmt.Println(v1)

	fmt.Println("V2")
	fmt.Println(v2)

	fmt.Println("---------- circle YAML Preview ---------")
	marshalled, err := yaml.Marshal(v2)
	if err != nil {
		fmt.Printf("Failed to Marshal v2 yml:\n %s", err)
	} else {
		fmt.Println(string(marshalled))
	}
	fmt.Println("----------------------------------------")

	// @TODO (INFRA-3158): after translation, write marshalled YAML to .circleci/config.yml
	// @TODO (INFRA-3158): after translation, remove or rename circle.yml
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
		models.DockerImage{
			Image: primaryImage,
		},
	}
	// @TODO (INFRA-3159): Determine and add additional database image(s) needed
	dbImages := []models.DockerImage{}
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

	// Install awscli for ECR interactions (used in docker publish steps)
	addInstallAWSCLIStep(&v2)

	// Create directories that were automatically created in CircleCI 1.0
	addCreateCIArtifactDirsStep(&v2)

	// Set up .npmrc if needed (for using private npm packages)
	// @TODO (INFRA-3149): is the set of apps with NODE_APP_TYPE the same as apps with .npmrc_docker files?
	// (this is for when app has .npmrc_docker file)
	if appType == NODE_APP_TYPE {
		addSetupNPMRCStep(&v2)
		addNPMInstallStep(&v2)
	}

	// translate COMPILE & TEST steps
	translateCompileSteps(&v1, &v2)
	translateTestSteps(&v1, &v2)

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

// @TODO (INFRA-3157): remove line breaks in deploy steps -- they break dapple deploy, for instance
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

func determineWorkingDirectory(appType string) (string, error) {
	// @TODO: determine decent working directory depending on app type for non-(go, wag, node) apps
	// go, wag: /go/src/github.com/Clever/catapult
	// non-wag node: ~/Clever/hubble

	org := "Clever"

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

	// put together working directory string
	if appType == GOLANG_APP_TYPE || appType == WAG_APP_TYPE {
		return fmt.Sprintf("/go/src/github.com/%s/%s", org, repo), nil
	}
	return fmt.Sprintf("~/%s/%s", org, repo), nil
}

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
	return imageConstraints
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

func getImage(constraints models.ImageConstraints) string {
	// @TODO (INFRA-3163): add human-readable image tags/other comments for image, if doable in yaml
	appType := constraints.AppType
	version := constraints.Version
	// default image (reproduces CircleCI 1.0 base)
	defaultImage := "circleci/build-image:ubuntu-14.04-XXL-upstart-1189-5614f37"
	if appType == GOLANG_APP_TYPE {
		// @TODO (INFRA-3149): base image for go and wag 1.8 or earlier that's not just the xxl default?
		if version == "1.10" {
			// circleci/golang:1.10.3-stretch
			// return "circleci/golang@sha256:4614481a383e55eef504f26f383db1329c285099fde0cfd342c49e5bb9b6c32a"
			return "circleci/golang:1.10.3-stretch"
		} else if version == "1.9" {
			// circleci/golang:1.9.7-stretch
			// return "circleci/golang@sha256:c46bee0b60747525d354f219083a46e06c68152f90f3bfb2812d1f232e6a5097"
			return "circleci/golang:1.9.7-stretch"
		}
	} else if appType == WAG_APP_TYPE {
		// @TODO (INFRA-3149): node version for wag locked in at 8.11.3 by these images -- could be ok (?)
		// @TODO (INFRA-3149): base image for node <6 that's not just the xxl default?
		if version == "1.10" {
			// circleci/golang:1.10.3-stretch
			// return "circleci/golang@sha256:4614481a383e55eef504f26f383db1329c285099fde0cfd342c49e5bb9b6c32a"
			return "circleci/golang:1.10.3-stretch"
		} else if version == "1.9" {
			// circleci/golang:1.9.7-stretch
			// return "circleci/golang@sha256:c46bee0b60747525d354f219083a46e06c68152f90f3bfb2812d1f232e6a5097"
			return "circleci/golang:1.9.7-stretch"
		}
	} else if appType == NODE_APP_TYPE {
		if version == "10" {
			return "circleci/node:10.8.0-stretch"
		} else if version == "8" {
			// circleci/node:8.11.3-stretch
			return "circleci/node:8.11.3-stretch"
		} else if version == "6" {
			// circleci/node:6.14.3-stretch
			return "circleci/node:6.14.3-stretch"
		}
	}
	fmt.Printf("No circleci image selected for app type %s, version %s -- using default\n", constraints.AppType, constraints.Version)
	return defaultImage
}
