package cli

import "github.com/andreyvit/rocketwind"

const tailwindBanner = "/*! tailwindcss v4.3.4 | MIT License | https://tailwindcss.com */\n"

type bannerCache struct {
	input, output       string
	inputMap, outputMap *rocketwind.SourceMap
	ready               bool
}

func (c *bannerCache) run(input string, inputMap *rocketwind.SourceMap) (string, *rocketwind.SourceMap) {
	if !c.ready || input != c.input || inputMap != c.inputMap {
		c.input, c.inputMap = input, inputMap
		c.output, c.outputMap = tailwindBanner+input, nil
		if inputMap != nil {
			copy := *inputMap
			copy.Mappings = ";" + copy.Mappings
			c.outputMap = &copy
		}
		c.ready = true
	}
	return c.output, c.outputMap
}
