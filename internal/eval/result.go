// Package eval evaluates compiled models exactly (BDD) or approximately (Monte Carlo) and defines the
// result types shared by the analysis, report and WASM layers.
package eval

// SchemaVersion is bumped on any breaking change to the JSON shapes below.
const SchemaVersion = 1

// Verdicts.
const (
	Achievable = "achievable"
	Marginal   = "marginal"
	Infeasible = "infeasible"
)

// Evaluation methods.
const (
	MethodBDD        = "bdd"
	MethodMonteCarlo = "monte_carlo"
)

// Result is everything `check` knows. Availabilities are fractions in [0, 1].
type Result struct {
	SchemaVersion int     `json:"schema_version"`
	Service       string  `json:"service"`
	Target        float64 `json:"target"`
	WindowDays    int     `json:"window_days"`
	Method        string  `json:"method"`
	BDDNodes      int     `json:"bdd_nodes,omitempty"`
	Availability  float64 `json:"availability"`
	// AvailabilityCI is the 95% confidence interval, Monte Carlo only.
	AvailabilityCI *[2]float64 `json:"availability_ci,omitempty"`
	// Naive is what multiplying percentages down the tree would say.
	Naive *float64 `json:"naive,omitempty"`
	// Ceiling is the availability with everything owned made perfect; nil when nothing is owned.
	Ceiling     *float64     `json:"ceiling"`
	Verdict     string       `json:"verdict"`
	Tier        *float64     `json:"tier"`
	Uncertainty *Uncertainty `json:"uncertainty,omitempty"`
	Levers      []Lever      `json:"levers"`
	SPOFs       []string     `json:"spofs"`
	CutSets     [][]string   `json:"cut_sets"`
	// CutSetsTruncated is true when more order-2 cut sets exist than are listed.
	CutSetsTruncated bool     `json:"cut_sets_truncated,omitempty"`
	Warnings         []string `json:"warnings"`
	Recommendation   string   `json:"recommendation"`
}

// Uncertainty summarises re-evaluating the same BDD under sampled component availabilities.
type Uncertainty struct {
	Samples      int     `json:"samples"`
	Seed         uint64  `json:"seed"`
	Mean         float64 `json:"mean"`
	P5           float64 `json:"p5"`
	P50          float64 `json:"p50"`
	P95          float64 `json:"p95"`
	PMeetsTarget float64 `json:"p_meets_target"`
}

// Lever ranks one component by how much improving it would help.
type Lever struct {
	Component    string  `json:"component"`
	Availability float64 `json:"availability"`
	Owned        bool    `json:"owned"`
	// Importance is Birnbaum: P(system | component perfect) - P(system | component failed).
	Importance float64 `json:"importance"`
	// ImprovementPotential is the exact gain from making the component perfect: P(system | perfect) - P(system).
	// It equals Importance * (1 - Availability) for a component with a single variable; a component with a
	// transient fraction owns several variables, where that product would overstate the gain.
	ImprovementPotential float64 `json:"improvement_potential"`
}

// Trace is the derivation tree printed by `nines explain`.
type Trace struct {
	SchemaVersion int        `json:"schema_version"`
	Service       string     `json:"service"`
	Root          *TraceNode `json:"root"`
}

// TraceNode is one node of the requires tree with its exact probability.
type TraceNode struct {
	Kind        string  `json:"kind"`
	Label       string  `json:"label"`
	Probability float64 `json:"probability"`
	// Shared lists components that appear under more than one child, so child values are not independent.
	Shared   []string     `json:"shared_components,omitempty"`
	Children []*TraceNode `json:"children,omitempty"`
}
