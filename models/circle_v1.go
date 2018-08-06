package models

// TODO:
///   - Support command modifiers (timeout, pwd, environment, parallel, files, background)

// CircleYamlV1
type CircleYamlV1 struct {
	Machine      MachinePhase                  `yaml:"machine,omitempty"`
	Checkout     Phase                         `yaml:"checkout,omitempty"`
	Dependencies Phase                         `yaml:"dependencies,omitempty"`
	Database     Phase                         `yaml:"database,omitempty"`
	Compile      Phase                         `yaml:"compile,omitempty"`
	Test         Phase                         `yaml:"test,omitempty"`
	Deployment   map[string]DeploymentSettings `yaml:"deployment,omitempty"`
	Notify       NotificationSettings          `yaml:"notify,omitempty"`
	General      GeneralSettings               `yaml:"general,omitempty"`
	// Experimental ExperimentalSettings `yaml:"experimental,omitempty"`  // TODO: implement if necessary
}

// MachinePhase
type MachinePhase struct {
	Pre         []string          `yaml:"pre,omitempty"`
	Post        []string          `yaml:"post,omitempty"`
	Environment map[string]string `yaml:"environment,omitempty"`
	Timezone    string            `yaml:"timezone,omitempty"`
	Hosts       map[string]string `yaml:"hosts,omitempty"`
	Ruby        VersionInfo       `yaml:"ruby,omitempty"`
	Node        VersionInfo       `yaml:"node,omitempty"`
	Java        VersionInfo       `yaml:"java,omitempty"`
	PHP         VersionInfo       `yaml:"php,omitempty"`
	Python      VersionInfo       `yaml:"python,omitempty"`
	GHC         VersionInfo       `yaml:"ghc,omitempty"`
	Services    []string          `yaml:"services,omitempty"`
}

// VersionInfo is the version of a language installed on the machine
type VersionInfo struct {
	Version string `yaml:"version,omitempty"`
}

// Phase is made up of 3 steps: pre (before), override (during), and post (after)
type Phase struct {
	Pre      []string `yaml:"pre,omitempty"`
	Override []string `yaml:"override,omitempty"`
	Post     []string `yaml:"post,omitempty"`
}

// DeploymentSettings configures when and how to deploy (after tests)
type DeploymentSettings struct {
	Branch   string   `yaml:"branch,omitempty"`
	Owner    string   `yaml:"owner,omitempty"`
	Commands []string `yaml:"commands,omitempty"`
}

// NotificationSettings configures which webhooks to trigger after tests are complete
type NotificationSettings struct {
	Webhooks []string `yaml:"webhooks,omitempty"` // TODO: other types of notifications
}

// GeneralSettings
type GeneralSettings struct {
	Branches  BranchSettings `yaml:"branches,omitempty"`
	BuildDir  string         `yaml:"build_dir,omitempty"`
	Artifacts []string       `yaml:"artifacts,omitempty"`
}

// BranchSettings
type BranchSettings struct {
	Ignore []string `yaml:"ignore,omitempty"` // TODO: these settings should be mutually exclusive
	Only   []string `yaml:"only,omitempty"`   ///      so we should make sure whichever is not set is empty
}
