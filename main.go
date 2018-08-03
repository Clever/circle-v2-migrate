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

const GOLANG = "go"
const NODE = "node"
const WAG = "wag"

// https://circleci.com/docs/2.0/migrating-from-1-2/

// Open V1 File

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

func convertToV2(v1 models.CircleYamlV1) (models.CircleYamlV2, error) {
	v2 := models.CircleYamlV2{
		Version: 2,
	}
	var imageNeeds string
	// @TODO: dynamically determine imageNeeds
	// if node, will have package.json and node.mk (but this is clever-specific) in main project dir
	// if go, will have golang.mk (but this is clever-specific)
	// another common occurance is go with node, which for us is mostly wag
	if _, err := os.Stat("./swagger.yml"); err == nil {
		imageNeeds = WAG
	} else if _, err := os.Stat("./golang.mk"); err == nil {
		imageNeeds = GOLANG
	} else if _, err := os.Stat("./node.mk"); err == nil {
		imageNeeds = NODE
	}
	fmt.Printf("!!!!!!!!!!!!!IMAGE NEEDS: %s\n", imageNeeds)

	// @TODO: dynamically determine org/repo
	// for go, both can be found from path to working directory where this script is running
	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	// /Users/briannaveenstra/go/src/github.com/Clever/catapult
	splitDir := strings.Split(dir, "/")
	var org string
	var repo string

	if len(splitDir) < 2 {
		log.Fatal(fmt.Errorf("failed to find org and repo in %s", dir))
	}
	org = splitDir[len(splitDir)-2]  // "Clever"
	repo = splitDir[len(splitDir)-1] // "catapult"

	// @TODO: Determine working dir depending on imageNeeds
	v2.Jobs.Build.WorkingDirectory = fmt.Sprintf("/go/src/github.com/%s/%s", org, repo)

	// @TODO: Determine image version based on makefile
	// for go, this is in makefile on line that says:
	// $(eval $(call golang-version-check,1.10))
	versionCheckRegexp, err := regexp.Compile(`golang-version-check,([0-1].[0-9]+)`)
	if err != nil {
		fmt.Errorf("error compiling regexp %s", err.Error())
	}

	makefile, err := ioutil.ReadFile("Makefile")
	if err != nil {
		log.Fatal(err)
	}
	versionCheck := versionCheckRegexp.FindSubmatch(makefile)
	image := "circleci/golang:1.08"
	if versionCheck != nil {
		version := string(versionCheck[1])
		image = fmt.Sprintf("circleci/golang:%s", version)
	}

	v2.Jobs.Build.Docker = []models.Docker{
		models.Docker{
			// @TODO: dynamically determine image (including go/node version)
			Image: image,
		},
	}

	v2.Jobs.Build.Environment = map[string]string{
		"CIRCLE_ARTIFACTS":    "/tmp/circleci-artifacts",
		"CIRCLE_TEST_REPORTS": "/tmp/circleci-test-results",
	}

	// Determine main setup
	for _, item := range v1.Machine.Services {
		if item == "docker" {
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "setup_remote_docker")
		}
	}

	// Add env vars for directories that were automatically generated in CircleCI 1.0
	ciArtifactsDirsStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "Set up CircleCI artifacts directories",
			"command": `mkdir -p $CIRCLE_ARTIFACTS $CIRCLE_TEST_REPORTS`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, ciArtifactsDirsStep)

	// Checkout repo
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "checkout")

	// Clone ci-scripts
	cloneCIScriptsStep := map[string]interface{}{
		"run": map[string]string{
			"name":    "Clone ci-scripts",
			"command": `cd $HOME && git clone --depth 1 -v https://github.com/Clever/ci-scripts.git && cd ci-scripts && git show --oneline -s`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, cloneCIScriptsStep)

	// Install awscli for ECR interactions (used in docker publish steps)
	installAWSCLIStep := map[string]interface{}{
		"run": map[string]string{
			"name": "Install awscli for ECR publish",
			"command": `cd /tmp/ && wget https://bootstrap.pypa.io/get-pip.py && sudo python get-pip.py
sudo pip install --upgrade awscli && aws --version
pip install --upgrade --user awscli`,
		},
	}
	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, installAWSCLIStep)

	////////////////////
	// COMPILE
	////////////////////
	// Determine build + test steps
	for _, item := range v1.Compile.Pre {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}

	for _, item := range v1.Compile.Override {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Compile.Post {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}

	////////////////////
	// TEST
	////////////////////
	for _, item := range v1.Test.Pre {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Test.Override {
		// v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, fmt.Sprintf("run: %s", item))
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}
	for _, item := range v1.Test.Post {
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, map[string]string{"run": item})
	}

	////////////////////
	// DEPLOYMENT
	////////////////////
	for key := range v1.Deployment {
		if key != "master" && key != "non-master" {
			return models.CircleYamlV2{}, fmt.Errorf("unexpected key in `deployment` map = %s", key)
		}
	}

	nonMaster, nonMasterOk := v1.Deployment["non-master"]
	master, masterOk := v1.Deployment["master"]

	// TODO: Find overlapping commands and remove IF (e.g. docker-publish / catapult-publish)
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

	return v2, nil
}
