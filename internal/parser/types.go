package parser

// Variable represents a parsed variable from defaults/main.yml.
type Variable struct {
	Name        string   // Variable name (e.g., "plex_role_web_subdomain")
	RawValue    string   // Original value as raw string, preserving formatting
	Type        string   // Inferred type (bool, string, list, dict, etc.)
	Section     string   // Main section name (e.g., "Docker")
	Subsection  string   // Subsection name if any
	Comment     string   // Associated comment text
	IsMultiline bool     // Whether value spans multiple lines
	ValueLines  []string // Individual lines for multiline values
	LineNumber  int      // Line number in source file
}

// Section represents a section of variables in the defaults file.
type Section struct {
	Name            string                // Section name (e.g., "Basics", "Docker")
	Variables       []Variable            // Variables directly in this section
	Subsections     map[string][]Variable // Subsection name -> variables
	SubsectionOrder []string              // Ordered list of subsection names
}

// RoleInfo contains all parsed information about a role.
type RoleInfo struct {
	Name           string              // Role name (e.g., "plex")
	RepoType       string              // "saltbox" or "sandbox"
	Sections       map[string]*Section // Section name -> Section
	SectionOrder   []string            // Ordered list of section names
	HasInstances   bool                // Whether role supports multiple instances
	InstancesVar   string              // Name of instances variable (e.g., "plex_instances")
	HasDefaultVars bool                // Whether role has _default/_custom vars
	SSOEnabled     bool                // Whether role has SSO enabled by default
	HasDNS         bool                // Whether role has a DNS section
	HasTraefik     bool                // Whether role has a Traefik section
	HasDocker      bool                // Whether role has a Docker section
	HasWeb         bool                // Whether role has a Web section
	HasThemePark   bool                // Whether role has ThemePark variables
	AllVariables   []Variable          // Flat list of all variables
}

// ParserState tracks the current parsing context.
type ParserState struct {
	CurrentSection    string
	CurrentSubsection string
	PendingComment    string
	GlobalComment     string
	InSubsection      bool
	SkipSection       bool // True when current section should be excluded from docs
}
