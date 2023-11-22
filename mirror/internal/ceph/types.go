package ceph

type peer struct {
	UUID        string `json:"uuid,omitempty"`
	ClusterName string `json:"cluster_name,omitempty"`
	ClientName  string `json:"client_name,omitempty"`
}

type peerInfoResult struct {
	Mode     string  `json:"mode,omitempty"`
	SiteName string  `json:"site_name,omitempty"`
	Peers    []*peer `json:"peers,omitempty"`
}

type mirroringInfo struct {
	State    string `json:"state,omitempty"`
	GlobalID string `json:"global_id,omitempty"`
	Primary  bool   `json:"primary,omitempty"`
}

type rbdImageInfo struct {
	Name      string   `json:"name,omitempty"`
	Features  []string `json:"features,omitempty"`
	Mirroring *mirroringInfo
}

type rbdMirrorStatusInfo struct {
	State       string `json:"state,omitempty"`
	Description string `json:"description,omitempty"`
}

type rbdImageSnapshotInfo struct {
	Name      string `json:"name,omitempty"`
	Protected string `json:"protected,omitempty"`
}
