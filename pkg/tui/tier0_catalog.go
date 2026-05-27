package tui

// Tier0Catalog matches the curated tests in ../../run-tier0.sh (14 entries).
// Each test uses a narrow --focus regex on a single It() spec, not a whole Describe block.
var Tier0Catalog = []TestSuite{
	{
		Name:  "cluster-creation-workflow",
		Focus: `Cluster Resource Type Lifecycle.*should validate complete workflow from creation to Reconciled state`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "cluster-creation-k8s-resources",
		Focus: `should create Kubernetes resources with correct templated values for adapters that create K8s resources`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "cluster-creation-adapter-dependency",
		Focus: `should validate cl-deployment dependency on cl-job with comprehensive condition checks`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "cluster-update",
		Focus:  `should update cluster via PATCH, trigger reconciliation, and reach Reconciled at new generation`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "cluster-delete-lifecycle",
		Focus: `Cluster Deletion Lifecycle.*should complete full deletion lifecycle from soft-delete through hard-delete`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "cluster-delete-conflict",
		Focus:  `should return 409 Conflict when PATCHing a soft-deleted cluster`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "cluster-cascade-delete",
		Focus: `should cascade deletion to child nodepools and hard-delete all resources`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "nodepool-creation-workflow",
		Focus: `NodePool Resource Type Lifecycle.*should validate complete workflow from creation to Reconciled state`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "nodepool-creation-k8s-resources",
		Focus: `should create Kubernetes resources with correct templated values for all required adapters`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "nodepool-update",
		Focus:  `should update nodepool via PATCH, trigger reconciliation, and reach Reconciled at new generation`,
		Labels: []string{"tier0"},
	},
	{
		Name:  "nodepool-delete-lifecycle",
		Focus: `NodePool Deletion Lifecycle.*should complete full deletion lifecycle from soft-delete through hard-delete`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "nodepool-delete-conflict",
		Focus:  `should return 409 Conflict when PATCHing a soft-deleted nodepool`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "adapter-maestro-happy-path",
		Focus:  `should create ManifestWork and report status via Maestro transport`,
		Labels: []string{"tier0"},
	},
	{
		Name:   "adapter-maestro-idempotency",
		Focus:  `should skip ManifestWork operation when generation is unchanged`,
		Labels: []string{"tier0"},
	},
}

// Tier0SuiteCount returns the number of tier0 tests (same as run-tier0.sh --list).
func Tier0SuiteCount() int { return len(Tier0Catalog) }
