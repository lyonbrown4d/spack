package config

import "strings"

type Robots struct {
	Enable    bool   `configx:"usage=Enable built-in robots.txt route generation."                 koanf:"enable"`
	Override  bool   `configx:"usage=Prefer generated robots.txt over a scanned robots.txt asset." koanf:"override"`
	UserAgent string `configx:"usage=Generated robots.txt User-agent value."                       koanf:"user_agent"`
	Allow     string `configx:"usage=Generated robots.txt Allow value."                            koanf:"allow"`
	Disallow  string `configx:"usage=Generated robots.txt Disallow value."                         koanf:"disallow"`
	Sitemap   string `configx:"usage=Generated robots.txt Sitemap value."                          koanf:"sitemap"`
	Host      string `configx:"usage=Generated robots.txt Host value."                             koanf:"host"`
}

func (r Robots) NormalizedUserAgent() string {
	userAgent := strings.TrimSpace(r.UserAgent)
	if userAgent == "" {
		return "*"
	}
	return userAgent
}
