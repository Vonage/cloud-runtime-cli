package deploy

// longText is the long description, byte for byte as the baseline prints it. The lines of the example manifest under
// security.override keep the tabs they have always had.
const longText = `Deploy your application to Vonage Cloud Runtime.

This command packages your application code and deploys it to the VCR platform.
The deployment process includes:
  1. Creating/retrieving your project
  2. Compressing and uploading your source code
  3. Building your application in the cloud
  4. Deploying to the specified region

MANIFEST FILE (vcr.yml)
  A manifest file defines your deployment configuration. The CLI looks for
  vcr.yml, vcr.yaml, neru.yml, or neru.yaml in your project directory.

  Example manifest:

  project:
    name: booking-app                    # Project name (lowercase, alphanumeric, hyphens)

  instance:
    name: dev                            # Instance name (e.g., dev, staging, prod)
    runtime: nodejs22                    # Runtime: nodejs22, nodejs18, python3, etc.
    region: aws.euw1                     # Region: aws.euw1, aws.use1, etc.
    application-id: <uuid>               # Vonage application UUID
    entrypoint:                          # Command to start your application
      - node
      - index.js
    environment:                         # Environment variables
      - name: VONAGE_NUMBER
        value: "12012010601"
      - name: API_KEY
        secret: MY_SECRET                # Reference a VCR secret that is created using the 'vcr secret create' command
    capabilities:                        # Vonage API capabilities
      - messages-v1
      - voice
      - rtc
    build-script: ./build.sh             # Optional build script
    domains:                             # Custom domains (optional)
      - api.example.com
    health-check-path: /custom/health    # Custom health check endpoint (default: /_/health)
    security:                            # Endpoint security (optional, defaults to public)
      access: private                    # Default access level: [private, public, authenticated]
      auth-method: vonage_basic           # Required when access is 'authenticated'
      override:
        - path: "/api/public"
          access: public                 # Override for specific paths
		- path: "/api/users/*/settings"
		  access: public
		- path: "/api/secure"
		  access: authenticated
		  auth-method: vonage_basic

  debug:
    name: debug                          # Debug instance name
    application-id: <uuid>               # Separate app for debugging (optional)
    entrypoint:
      - node
      - --inspect
      - index.js

APPLICATION REQUIREMENTS
  Your application must meet these requirements to deploy successfully:

  1. HTTP Server on VCR_PORT
     Your app MUST listen for HTTP requests on the port specified by the
     VCR_PORT environment variable (automatically injected by VCR).

     Example (Node.js):
       const port = process.env.VCR_PORT || 8080;
       app.listen(port, () => console.log('Server running on port ' + port));

     Example (Python):
       port = int(os.environ.get('VCR_PORT', 8080))
       app.run(host='0.0.0.0', port=port)

  2. Health Check Endpoint
     Your app MUST expose a health check endpoint that returns HTTP 200.
     VCR uses this to verify your app started correctly.

     By default, VCR checks GET /_/health. You can customize this by
     setting health-check-path in your manifest:

       instance:
         health-check-path: /custom/health

     Example (Node.js/Express):
       app.get('/_/health', (req, res) => res.status(200).send('OK'));

     Example (Python/Flask):
       @app.route('/_/health')
       def health(): return 'OK', 200

IGNORING FILES
  Create a .vcrignore file to exclude files from deployment (similar to .gitignore).
  Common exclusions: node_modules/, .git/, *.log, .env

CAPABILITIES
  • messages-v1  - Messages API (SMS, WhatsApp, Viber, etc.)
  • voice        - Voice API (phone calls, IVR)
  • rtc          - Real-Time Communication (in-app voice/video)

SECURITY ACCESS LEVELS
  • public          - No authentication required (default if security is omitted)
  • private         - Returns forbidden for those paths
  • authenticated   - Requires authentication using a specified auth method

AUTH METHODS (used with 'authenticated' access)
  • vonage_basic    - Vonage Basic authentication

TROUBLESHOOTING
  "credential not found" error after deployment:
    This usually means the Vonage application keys need to be regenerated
    for VCR access. Run:
      $ vcr app generate-keys --app-id <your-app-id>

  Application fails to start:
    • Verify your app listens on VCR_PORT (not a hardcoded port)
    • Ensure GET /_/health returns HTTP 200
    • Check logs with: vcr instance log -p <project> -n <instance>
`

// exampleText is the example block of the help page.
const exampleText = `# Deploy from the current directory
$ vcr deploy

# Deploy from a specific directory
$ vcr deploy ./my-project

# Override the project name
$ vcr deploy --project-name my-project

# Override the instance name
$ vcr deploy --instance-name production

# Override the runtime
$ vcr deploy --runtime nodejs20

# Override the Vonage application ID
$ vcr deploy --app-id 12345678-1234-1234-1234-123456789abc

# Deploy a pre-compressed tarball
$ vcr deploy --tgz ./my-app.tar.gz

# Use a custom manifest file
$ vcr deploy --filename ./custom-manifest.yml

# Override capabilities
$ vcr deploy --capabilities "messages-v1,voice"
`
