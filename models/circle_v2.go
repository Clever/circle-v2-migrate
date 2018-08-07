package models

// CircleYamlV2
type CircleYamlV2 struct {
	Version int `yaml:"version,omitempty"`
	Jobs    struct {
		Build struct {
			WorkingDirectory string            `yaml:"working_directory,omitempty"`
			Docker           []DockerImage     `yaml:"docker,omitempty"`
			Environment      map[string]string `yaml:"environment,omitempty"`
			Steps            []interface{}     `yaml:"steps,omitempty"`
		} `yaml:"build,omitempty"`
	} `yaml:"jobs,omitempty"`
}

type DockerImage struct {
	Image string `yaml:"image,omitempty"`
}
