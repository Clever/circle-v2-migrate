package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/Clever/circle-v2-migrate/models"
	yaml "gopkg.in/yaml.v2"
)

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

	// TODO: dynamically determine org/repo
	org := "Clever"
	repo := "kinesis-to-firehose"

	// TODO: Determine working dir depending on language
	v2.Jobs.Build.WorkingDirectory = fmt.Sprintf("/go/src/github.com/%s/%s", org, repo)
	v2.Jobs.Build.Docker = []models.Docker{
		models.Docker{
			Image: "circleci/golang:1.8",
		},
	}

	v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "checkout")

	// v1.Dependencies
	// v1.Database
	// v1.General

	// Determine language and main setup
	for _, item := range v1.Machine.Services {
		if item == "docker" {
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, "setup_remote_docker")
		}
	}

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
		step := map[string]string{"deploy": item}
		v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
	}

	if nonMasterOk {
		for _, item := range nonMaster.Commands {
			if _, isDuplicate := overlap[item]; isDuplicate {
				continue
			}

			step := map[string]string{"deploy": `if [ "${CIRCLE_BRANCH}" != "master" ]; then ` + item + `; fi;`}
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
		}
	}

	if masterOk {
		for _, item := range master.Commands {
			if _, isDuplicate := overlap[item]; isDuplicate {
				continue
			}

			step := map[string]string{"deploy": `if [ "${CIRCLE_BRANCH}" == "master" ]; then ` + item + `; fi;`}
			v2.Jobs.Build.Steps = append(v2.Jobs.Build.Steps, step)
		}
	}

	return v2, nil
}
