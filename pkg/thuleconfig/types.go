package thuleconfig

// Config defines project-level thule.yaml settings.
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
	Helm Helm   `json:"helm,omitempty"`
	Flux Flux   `json:"flux,omitempty"`
}

type Helm struct {
	ReleaseName string   `json:"releaseName,omitempty"`
	ValuesFiles []string `json:"valuesFiles,omitempty"`
}

type Flux struct {
	IncludeKinds []string `json:"includeKinds,omitempty"`
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
