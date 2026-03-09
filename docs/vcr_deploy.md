## vcr deploy

Deploy a VCR application

### Synopsis

Deploy a VCR application.

This command will package up the local client app code and deploy it to the VCR platform.

A deployment manifest should be provided so that the CLI knows how to deploy your application. An example manifest would look like:

project:
	name: booking-app
instance:
	name: dev
	runtime: nodejs18
	region: aws.euw1
	application-id: 0dcbb945-cf09-4756-808a-e1873228f802
	environment:
		- name: VONAGE_NUMBER
		  value: "12012010601"
    capabilities:
		- messages-v1
		- rtc
	entrypoint:
		- node
		- index.js
	health-check-path: /custom/health
	security:
		access: private
		override:
			- path: "/api/public"
			  access: public
			- path: "/v1/users/*/profile"
			  access: public
			- path: "/api/secure"
			  access: authenticated
			  auth-method: vonage_basic
debug:
	name: debug
	application-id: 0dcbb945-cf09-4756-808a-e1873228f802
	environment:
		- name: VONAGE_NUMBER
		  value: "12012010601"
	entrypoint:
		- node
		- index.js

By default, the CLI will look for a deployment manifest in the root of the code directory under the name 'vcr.yml'.
Flags can be used to override the mandatory fields, ie project name, instance name, runtime, region and application ID.

The project will be created if it does not already exist.

#### Health Check Path

Your application must expose a health check endpoint that returns HTTP 200. VCR uses this to verify your app started correctly.

By default, VCR checks `GET /_/health`. You can customize this by setting `health-check-path` in your manifest:

```yaml
instance:
  health-check-path: /custom/health
```

If not specified, the default `/_/health` path is used.

#### Security Configuration

The `security` configuration allows you to control access to your application and specific paths:

- **public**: Allows public access to reach those paths
- **private**: Returns forbidden for those paths
- **authenticated**: Requires authentication to access those paths. Must be used with `auth-method`

**Default Behavior:**
If no `security` field is specified in your manifest, all endpoints will default to public access.

**Configuration Structure:**
- `access`: Sets the default access level for all paths ("public", "private", or "authenticated")
- `auth-method`: (Required when `access` is "authenticated") Sets the authentication method. The only supported value is `"vonage_basic"`. Not allowed for "public" or "private" access
- `override`: Array of path-specific access overrides
  - `path`: The path pattern to override
  - `access`: The access level for this path ("public", "private", or "authenticated")
  - `auth-method`: (Required when `access` is "authenticated") The authentication method for this specific path. The only supported value is `"vonage_basic"`

**Wildcard Support:**
- Use `*` to match a single path segment: `/v1/users/*/settings`
- Use `**` to match multiple path segments: `/v1/**`

**Examples:**
```yaml
# Example 1: Default public with specific private paths
security:
  access: public
  override:
    - path: "/api/admin"
      access: private
    - path: "/v1/internal/**"
      access: private

# Example 2: Default private with specific public paths
security:
  access: private
  override:
    - path: "/api/health"
      access: public
    - path: "/v1/users/*/profile"
      access: public

# Example 3: Authenticated access with vonage_basic
security:
  access: authenticated
  auth-method: vonage_basic
  override:
    - path: "/api/public"
      access: public
    - path: "/api/admin"
      access: authenticated
      auth-method: vonage_basic
```


```
vcr deploy [path_to_code] [flags]
```

### Examples

```
# Deploy code in current app directory.
$ vcr deploy .
		
# If no arguments are provided, the code directory is assumed to be the current directory.
$ vcr deploy

```

### Options

```
  -i, --app-id string          Set the id of the Vonage application you wish to link the VCR application to
  -c, --capabilities string    Provide the comma separated capabilities required for your application. eg: "messaging,voice"
  -f, --filename string        File contains the VCR manifest to apply
  -n, --instance-name string   Instance name
  -p, --project-name string    Project name
  -r, --runtime string         Set the runtime of the application
  -z, --tgz string             Provide the path to the tar.gz code you wish to deploy. Code need to be compressed from root directory and include library
```

### Options inherited from parent commands

```
      --api-key string            Vonage API key
      --api-secret string         Vonage API secret
      --config-file string        Path to config file (default is $HOME/.vcr-cli) (default "~/.vcr-cli")
      --graphql-endpoint string   Graphql endpoint used to fetch metadata
      --help                      Show help for command
      --region string             Vonage platform region
  -t, --timeout duration          Timeout for requests to Vonage platform (default 10m0s)
```

### SEE ALSO

* [vcr](vcr.md)	 - Streamline your Vonage Cloud Runtime development and management tasks with VCR

###### Auto generated by spf13/cobra on 26-Nov-2024
