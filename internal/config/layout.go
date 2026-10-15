package config

import "gopkg.in/ini.v1"

const (
	sectionCredentials = "credentials"
	keyGraphQLEndpoint = "graphql_endpoint"
	keyDefaultRegion   = "default_region"
	keyAPIKey          = "api_key"
	keyAPISecret       = "api_secret"
)

// fileFrom picks the four known values out of a parsed document. Anything else in it is ignored.
func fileFrom(doc *ini.File) File {
	top, credentials := doc.Section(""), doc.Section(sectionCredentials)
	return File{
		GraphQLEndpoint: valueOf(top, keyGraphQLEndpoint),
		DefaultRegion:   valueOf(top, keyDefaultRegion),
		APIKey:          valueOf(credentials, keyAPIKey),
		APISecret:       valueOf(credentials, keyAPISecret),
	}
}

// valueOf reads a key the way the library maps it into a struct, so values come out exactly as they always have.
func valueOf(section *ini.Section, name string) string {
	key, err := section.GetKey(name)
	if err != nil {
		return ""
	}
	return key.String()
}

// applyTo sets the four known values, adding the credentials section when the document has none.
func applyTo(doc *ini.File, f File) {
	top, credentials := doc.Section(""), doc.Section(sectionCredentials)
	top.Key(keyGraphQLEndpoint).SetValue(f.GraphQLEndpoint)
	top.Key(keyDefaultRegion).SetValue(f.DefaultRegion)
	credentials.Key(keyAPIKey).SetValue(f.APIKey)
	credentials.Key(keyAPISecret).SetValue(f.APISecret)
}
