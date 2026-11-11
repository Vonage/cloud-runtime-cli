package main

const schemaID = "https://raw.githubusercontent.com/Vonage/cloud-runtime-cli/main/schema/vcr.schema.json"

// manifestSchema is the schema of vcr.yml. It mirrors the structs and yaml tags in internal/manifest/manifest.go;
// caps, generated from the Vonage Application API v2 definition, is application-capabilities.
func manifestSchema(caps map[string]any) map[string]any {
	s := object("The Vonage Cloud Runtime manifest that the vcr CLI reads.", map[string]any{
		"project": object("The project.", map[string]any{
			"name": str("The project name."),
		}),
		"instance": object("The instance that vcr deploy creates or updates.", map[string]any{
			"name":           str("The instance name."),
			"runtime":        str("The runtime, for example nodejs22."),
			"region":         str("The region, for example aws.euw1."),
			"application-id": str("The ID of the Vonage application the instance uses."),
			"environment":    environment("Environment variables of the deployed instance."),
			"capabilities": strList("The Vonage capabilities to enable, for example voice or messages-v1. " +
				"application-capabilities replaces this list."),
			"entrypoint":   strList("The command that starts the application."),
			"domains":      strList("Custom domains for the instance."),
			"build-script": str("A script run before the code is uploaded."),
			"scaling": object("The instance's scaling limits.", map[string]any{
				"min-scale": integer("The smallest number of replicas."),
				"max-scale": integer("The largest number of replicas."),
			}),
			"security": object("Who can call the instance.", map[string]any{
				"access":      str("The access level, for example public, private or authenticated."),
				"auth-method": str("The authentication method, for example vonage_basic."),
				"override": list("Access levels for single paths.", object("The access level of one path.", map[string]any{
					"path":        str("The path, for example /api/public."),
					"access":      str("The access level of the path."),
					"auth-method": str("The authentication method of the path."),
				})),
			}),
			"health-check-path": str("The path the platform calls to check the instance's health."),
		}),
		"debug": object("The settings vcr debug uses.", map[string]any{
			"name":           str("The debug instance name."),
			"application-id": str("The ID of the Vonage application the debug session uses."),
			"environment":    environment("Environment variables of the debug session."),
			"entrypoint":     strList("The command that starts the application locally."),
			"preserve-data":  map[string]any{"type": "boolean", "description": "Keep the session data after it stops."},
		}),
		"application-capabilities": caps,
	})
	s["$schema"] = "http://json-schema.org/draft-07/schema#"
	s["$id"] = schemaID
	s["title"] = "vcr.yml"
	return s
}

func object(description string, properties map[string]any) map[string]any {
	return map[string]any{
		"type": "object", "description": description, "properties": properties, "additionalProperties": true,
	}
}

func str(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integer(description string) map[string]any {
	return map[string]any{"type": "integer", "description": description}
}

func list(description string, items map[string]any) map[string]any {
	return map[string]any{"type": "array", "description": description, "items": items}
}

func strList(description string) map[string]any {
	return list(description, map[string]any{"type": "string"})
}

func environment(description string) map[string]any {
	return list(description, object("One environment variable.", map[string]any{
		"name": str("The variable name."),
		// The CLI reads an unquoted number or boolean as its text.
		"value":  map[string]any{"type": []any{"string", "number", "boolean"}, "description": "The value."},
		"secret": str("The name of a secret that holds the value."),
	}))
}
