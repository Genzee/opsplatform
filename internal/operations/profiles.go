package operations

// These are the explicitly supported M1 semantic profiles. Unknown facts remain
// discoverable through neighbors/path but never acquire impact semantics by name.
type rule struct {
	source, edge, target string
	reverse              bool
}

var supportRules = []rule{
	{"kubernetes.Deployment", "OWNS", "kubernetes.ReplicaSet", false},
	{"kubernetes.ReplicaSet", "OWNS", "kubernetes.Pod", false},
	{"kubernetes.StatefulSet", "OWNS", "kubernetes.Pod", false},
	{"kubernetes.DaemonSet", "OWNS", "kubernetes.Pod", false},
	{"kubernetes.Pod", "CONTAINS", "kubernetes.Container", false},
	{"kubernetes.Pod", "RUNS_ON", "kubernetes.Node", false},
	{"kubernetes.Container", "RUNS_ON", "kubernetes.Node", false},
	{"kubernetes.Node", "BACKED_BY", "linux.OS", false},
	{"kubernetes.Container", "BACKED_BY", "linux.Cgroup", false},
	{"kubernetes.Pod", "USES", "kubernetes.PersistentVolumeClaim", false},
	{"kubernetes.Container", "MOUNTS", "kubernetes.PersistentVolumeClaim", false},
	{"kubernetes.PersistentVolumeClaim", "BOUND_TO", "kubernetes.PersistentVolume", false},
	{"linux.Mount", "BACKED_BY", "linux.Filesystem", false},
	{"linux.Filesystem", "BACKED_BY", "linux.BlockDevice", false},
	{"linux.Filesystem", "USES", "nfs.Server", false},
	{"kubernetes.Container", "MOUNTS", "linux.Mount", false},
	{"kubernetes.Service", "HAS_ENDPOINT_SLICE", "kubernetes.EndpointSlice", false},
	{"kubernetes.EndpointSlice", "REFERENCES", "kubernetes.Pod", false},
}

var inventoryRules = []rule{
	{"linux.OS", "HAS_INTERFACE", "linux.NetworkInterface", false},
	{"linux.OS", "HAS_INTERFACE", "linux.Bridge", false},
	{"linux.OS", "HAS_INTERFACE", "linux.Bond", false},
	{"linux.OS", "HAS_ROUTE", "linux.Route", false},
	{"linux.OS", "HAS_MOUNT", "linux.Mount", false},
	{"linux.OS", "HAS_DEVICE", "linux.BlockDevice", false},
	{"linux.OS", "RUNS", "linux.Process", false},
	{"linux.Cgroup", "CONTAINS", "linux.Process", false},
	{"linux.Process", "MEMBER_OF", "linux.Cgroup", false},
	{"linux.NetworkInterface", "HAS_ADDRESS", "linux.IPAddress", false},
	{"linux.Bridge", "HAS_ADDRESS", "linux.IPAddress", false},
	{"linux.Bond", "HAS_ADDRESS", "linux.IPAddress", false},
}

func permits(profile, direction, sourceKind, edgeType, targetKind string, reverse bool) bool {
	if profile == "neighbors" || profile == "path" {
		return direction == "both" || (direction == "in" && reverse) || (direction == "out" && !reverse)
	}
	wantReverse := profile == "potential-impact/v1"
	if reverse != wantReverse {
		return false
	}
	for _, r := range supportRules {
		if r.source == sourceKind && r.edge == edgeType && r.target == targetKind {
			return true
		}
	}
	if profile == "infrastructure/v1" {
		for _, r := range inventoryRules {
			if r.source == sourceKind && r.edge == edgeType && r.target == targetKind {
				return true
			}
		}
	}
	return false
}
