package cli

import "github.com/andreyvit/rocketwind"

// A watcher often receives the exact same CSS and map from its builder. Retain
// the optimized result until either changes; failed builds don't poison it.
type optimizationCache struct {
	options             rocketwind.OptimizeOptions
	input, output       string
	inputMap, outputMap *rocketwind.SourceMap
	ready               bool
}

func (c *optimizationCache) run(input string, inputMap *rocketwind.SourceMap) (string, *rocketwind.SourceMap, error) {
	if c.ready && input == c.input && inputMap == c.inputMap {
		return c.output, c.outputMap, nil
	}
	var output string
	var outputMap *rocketwind.SourceMap
	var err error
	if inputMap == nil {
		output, err = rocketwind.Optimize(input, c.options)
	} else {
		output, outputMap, err = rocketwind.OptimizeWithSourceMap(input, c.options, inputMap)
	}
	if err != nil {
		return "", nil, err
	}
	c.input, c.inputMap, c.output, c.outputMap, c.ready = input, inputMap, output, outputMap, true
	return output, outputMap, nil
}
