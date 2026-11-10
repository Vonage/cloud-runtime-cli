package cli

// rootLong and rootExample are the root help texts, byte for byte as the contract tree records them.
const rootLong = `VCR CLI is a powerful command-line interface designed to streamline
and simplify the development and management of applications on
the Vonage Cloud Runtime platform.

Vonage Cloud Runtime (VCR) enables you to build, deploy, and run serverless
applications that integrate with Vonage communication APIs including Voice,
Messages, and RTC (Real-Time Communication).

GETTING STARTED
  1. Configure the CLI with your Vonage API credentials:
     $ vcr configure

  2. Initialize a new project from a template:
     $ vcr init my-project

  3. Deploy your application:
     $ vcr deploy

CORE WORKFLOW
  • vcr configure  - Set up your Vonage API credentials and region
  • vcr app        - Create and manage Vonage applications
  • vcr init       - Initialize a project from a template
  • vcr deploy     - Deploy your application to VCR
  • vcr debug      - Run your application locally in debug mode
  • vcr instance   - Manage deployed instances (logs, removal)
  • vcr secret     - Manage secrets for your applications
  • vcr upgrade    - Update the VCR CLI to the latest version
`

const rootExample = `# Configure the CLI with your Vonage credentials
$ vcr configure

# Create a new Vonage application
$ vcr app create --name my-app

# List all your Vonage applications
$ vcr app list

# Initialize a new project in the current directory
$ vcr init

# Initialize a new project in a specific directory
$ vcr init my-project

# Deploy your application to VCR
$ vcr deploy

# Run your application locally in debug mode
$ vcr debug

# View logs for a deployed instance
$ vcr instance log --project-name my-project --instance-name dev

# Create a secret for your application
$ vcr secret create --name MY_API_KEY --value "secret-value"
`
