package thuleconfig

// Config defines project-level thule.yaml settings for Phase 0.
type Config struct {
	Version    string  `json:"version"`
	Project    string  `json:"project"`
	ClusterRef string  `json:"clusterRef"`
	Namespace  string  `json:"namespace"`
	Render     Render  `json:"render"`
	Diff       Diff    `json:"diff,omitempty"`
	Policy     Policy  `json:"policy,omitempty"`
	Comment    Comment `json:"comment,omitempty"`
}

type Render struct {
	Mode string `json:"mode"`
	Path string `json:"path"`
}

type Diff struct {
	Prune        bool     `json:"prune"`
	IgnoreFields []string `json:"ignoreFields,omitempty"`
}

type Policy struct {
	Profile string `json:"profile,omitempty"`
}

type Comment struct {
	MaxResourceDetails int `json:"maxResourceDetails,omitempty"`
}
