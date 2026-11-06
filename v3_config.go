package rocketwind

func (c *Compiler) finishV3Legacy() error {
	resolved := c.legacyConfig
	if prefix, ok := resolved.Get("prefix").(string); ok && prefix != "" {
		c.v3Prefix = prefix
	}
	if important, ok := resolved.Get("important").(bool); ok {
		c.config.Important = important
	}
	if selector, ok := resolved.Get("important").(string); ok {
		c.importantSelector = selector
	}
	if blocked, ok := resolved.Get("blocklist").([]any); ok {
		c.blocklist = map[string]bool{}
		for _, value := range blocked {
			if class, ok := value.(string); ok {
				c.blocklist[class] = true
			}
		}
	}
	c.configureDarkMode(resolved.Get("darkMode"))
	return nil
}
