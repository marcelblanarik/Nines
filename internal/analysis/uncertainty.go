package analysis

import (
	"math"
	"math/rand/v2"
	"sort"

	"github.com/marcelblanarik/nines/internal/compile"
	"github.com/marcelblanarik/nines/internal/eval"
	"github.com/marcelblanarik/nines/internal/model"
)

// uncertainty samples component availabilities (point: fixed, interval: uniform, counts: Beta(good+1,
// bad+1)) and re-evaluates the same BDD each time with new variable probabilities.
func uncertainty(c *compile.Compiled, e *eval.Engine, opt Options, target float64) *eval.Uncertainty {
	rng := eval.NewRand(opt.Seed)
	n := opt.samples()
	comps := make([]*model.Component, len(c.Components))
	for i, name := range c.Components {
		comps[i] = c.Model.Lookup(name)
	}
	avail := make(map[string]float64, len(comps))
	samples := make([]float64, n)
	sum, meets := 0.0, 0
	for s := range samples {
		for _, comp := range comps {
			avail[comp.Name] = sampleAvailability(rng, comp.Avail)
		}
		p := e.P(c.Probs(avail))
		samples[s] = p
		sum += p
		if p >= target-1e-12 {
			meets++
		}
	}
	sort.Float64s(samples)
	return &eval.Uncertainty{
		Samples:      n,
		Seed:         opt.Seed,
		Mean:         sum / float64(n),
		P5:           percentile(samples, 0.05),
		P50:          percentile(samples, 0.5),
		P95:          percentile(samples, 0.95),
		PMeetsTarget: float64(meets) / float64(n),
	}
}

func sampleAvailability(rng *rand.Rand, a model.Availability) float64 {
	switch a.Kind {
	case model.Interval:
		return (a.Lo + rng.Float64()*(a.Hi-a.Lo)) / 100
	case model.Counts:
		return beta(rng, float64(a.Good)+1, float64(a.Total-a.Good)+1)
	}
	return a.Percent / 100
}

// percentile of an ascending slice, interpolating between neighbours.
func percentile(sorted []float64, q float64) float64 {
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := min(lo+1, len(sorted)-1)
	return sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
}

func beta(rng *rand.Rand, a, b float64) float64 {
	x, y := gamma(rng, a), gamma(rng, b)
	return x / (x + y)
}

// gamma draws from Gamma(shape, 1) for shape >= 1 (Marsaglia and Tsang).
func gamma(rng *rand.Rand, shape float64) float64 {
	d := shape - 1.0/3
	c := 1 / math.Sqrt(9*d)
	for {
		x := rng.NormFloat64()
		v := 1 + c*x
		if v <= 0 {
			continue
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1-0.0331*x*x*x*x || math.Log(u) < 0.5*x*x+d*(1-v+math.Log(v)) {
			return d * v
		}
	}
}
