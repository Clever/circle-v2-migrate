package models

// CircleYamlV1
type CircleYamlV2 struct {
	Version int `yaml:"version,omitempty"`
	Jobs    struct {
		Build struct {
			WorkingDirectory string        `yaml:"working_directory,omitempty"`
			Docker           []Docker      `yaml:"docker,omitempty"`
			Steps            []interface{} `yaml:"steps,omitempty"`
		} `yaml:"build,omitempty"`
	} `yaml:"jobs,omitempty"`
}

type Docker struct {
	Image string `yaml:"image,omitempty"`
}
