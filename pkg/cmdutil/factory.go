package cmdutil

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/go-resty/resty/v2"
	"github.com/google/uuid"

	"vonage-cloud-runtime-cli/pkg/api"
	"vonage-cloud-runtime-cli/pkg/config"
)

//go:generate mockgen -source=factory.go -package mocks -destination ../../testutil/mocks/factory.go

type SurveyInterface interface {
	AskYesNo(question string) bool
	AskForUserInput(question string, defaultValue string) (string, error)
	AskForUserChoice(question string, choices []string, lookup map[string]string, defaultValue string) (string, error)
}

type AssetInterface interface {
	GetTemplateNameList(ctx context.Context, prefix string, isRecursive bool, limit int) ([]api.Metadata, error)
	GetTemplate(ctx context.Context, templateName string) (api.Template, error)
}

type ReleaseInterface interface {
	GetLatestRelease(ctx context.Context) (api.Release, error)
	GetAsset(ctx context.Context, url string) ([]byte, error)
}

type MarketplaceInterface interface {
	GetTemplate(ctx context.Context, productID, versionID string) ([]byte, error)
}

type DeploymentInterface interface {
	CreateVonageApplication(ctx context.Context, name string, enableRTC, enableVoice, enableMessages bool) (api.CreateVonageApplicationOutput, error)
	ListVonageApplications(ctx context.Context, filter string) (api.ListVonageApplicationsOutput, error)
	GenerateVonageApplicationKeys(ctx context.Context, appID string) error
	DeployDebugService(ctx context.Context, region, applicationID, name string, caps api.Capabilities) (api.DeployResponse, error)
	GetServiceReadyStatus(ctx context.Context, serviceName string) (bool, error)
	DeleteDebugService(ctx context.Context, serviceName string, preserveData bool) error
	CreatePackage(ctx context.Context, createPackageArgs api.CreatePackageArgs) (api.CreatePackageResponse, error)
	CreateProject(ctx context.Context, projectName string) (api.CreateProjectResponse, error)
	DeployInstance(ctx context.Context, deployInstanceArgs api.DeployInstanceArgs) (api.DeployInstanceResponse, error)
	DeleteInstance(ctx context.Context, instanceID string) error
	UploadTgz(ctx context.Context, fileBytes []byte) (api.UploadResponse, error)
	WatchDeployment(ctx context.Context, out *iostreams.IOStreams, packageID string) error
	CreateSecret(ctx context.Context, s config.Secret) error
	UpdateSecret(ctx context.Context, s config.Secret) error
	RemoveSecret(ctx context.Context, name string) error
	CreateMongoDatabase(ctx context.Context, version string) (api.MongoInfoResponse, error)
	DeleteMongoDatabase(ctx context.Context, version, database string) error
	GetMongoDatabase(ctx context.Context, version, database string) (api.MongoInfoResponse, error)
	ListMongoDatabases(ctx context.Context, version string) ([]string, error)
}

type DatastoreInterface interface {
	ListRegions(ctx context.Context) ([]api.Region, error)
	GetRegion(ctx context.Context, alias string) (api.Region, error)
	GetInstanceByProjectAndInstanceName(ctx context.Context, projectName, instanceName string) (api.Instance, error)
	GetInstanceByID(ctx context.Context, instanceID string) (api.Instance, error)
	ListRuntimes(ctx context.Context) ([]api.Runtime, error)
	GetRuntimeByName(ctx context.Context, name string) (api.Runtime, error)
	GetProject(ctx context.Context, accountID, name string) (api.Project, error)
	ListProducts(ctx context.Context) ([]api.Product, error)
	GetLatestProductVersionByID(ctx context.Context, id string) (api.ProductVersion, error)
}

// Factory provides clients and parameters for all subcommands.
type Factory interface {
	Init(ctx context.Context, cfg config.CLIConfig, opts *config.GlobalOptions) error
	InitDatastore(cfg config.CLIConfig, opts *config.GlobalOptions)
	InitDeploymentClient(ctx context.Context, regionAlias string) error
	SetGlobalOptions(opts *config.GlobalOptions)
	SetCliConfig(opts config.CLIConfig)
	IOStreams() *iostreams.IOStreams
	HTTPClient() *resty.Client
	AssetClient() AssetInterface
	ReleaseClient() ReleaseInterface
	MarketplaceClient() MarketplaceInterface
	Datastore() DatastoreInterface
	DeploymentClient() DeploymentInterface
	Survey() SurveyInterface
	ConfigFilePath() string
	GlobalOptions() *config.GlobalOptions
	CliConfig() config.CLIConfig
	APIKey() string
	APISecret() string
	Region() string
	GraphQLURL() string
	Deadline() time.Time
	Timeout() time.Duration
}

// DefaultFactory is the default implementation of Factory.
type DefaultFactory struct {
	ioStreams  *iostreams.IOStreams
	survey     *Survey
	globalOpts *config.GlobalOptions
	cliConfig  config.CLIConfig
	apiVersion string
	releaseURL string

	websocketConnectionClient *api.WebsocketConnectionClient
	httpClient                *resty.Client
	assetClient               *api.AssetClient
	deploymentClient          *api.DeploymentClient
	datastore                 *api.Datastore
	releaseClient             *api.ReleaseClient
	marketplaceClient         *api.MarketplaceClient
}

func NewDefaultFactory(apiVersion string, releaseURL string) *DefaultFactory {
	f := &DefaultFactory{
		ioStreams:  iostreams.System(),
		apiVersion: apiVersion,
		releaseURL: releaseURL,
		survey:     &Survey{},
	}
	return f
}

func (f *DefaultFactory) Init(ctx context.Context, cfg config.CLIConfig, opts *config.GlobalOptions) error {
	f.cliConfig = cfg
	f.globalOpts = opts
	f.websocketConnectionClient = getWebsocketConnectionClient(f.APIKey(), f.APISecret())
	f.httpClient = GetHTTPClient(f.APIKey(), f.APISecret())
	f.datastore = getDatastore(f.GraphQLURL(), f.httpClient)
	region, err := f.datastore.GetRegion(ctx, f.Region())
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("region does not exist")
		}
		return err
	}
	f.assetClient = api.NewAssetClient(region.AssetsAPIURL, f.httpClient)
	f.deploymentClient = api.NewDeploymentClient(region.DeploymentAPIURL, f.apiVersion, f.httpClient, f.websocketConnectionClient)
	f.releaseClient = api.NewReleaseClient(f.releaseURL, f.httpClient)
	f.marketplaceClient = api.NewMarketplaceClient(region.MarketplaceAPIURL, f.httpClient)
	return nil
}

func (f *DefaultFactory) InitDatastore(cfg config.CLIConfig, opts *config.GlobalOptions) {
	f.globalOpts = opts
	f.cliConfig = cfg
	f.httpClient = GetHTTPClient(f.APIKey(), f.APISecret())
	f.datastore = getDatastore(f.GraphQLURL(), f.httpClient)
}

func (f *DefaultFactory) InitDeploymentClient(ctx context.Context, regionAlias string) error {
	region, err := f.datastore.GetRegion(ctx, regionAlias)
	if err != nil {
		if errors.Is(err, api.ErrNotFound) {
			return fmt.Errorf("region does not exist")
		}
		return err
	}
	f.deploymentClient = api.NewDeploymentClient(region.DeploymentAPIURL, f.apiVersion, f.httpClient, f.websocketConnectionClient)
	return nil
}

func (f *DefaultFactory) SetGlobalOptions(opts *config.GlobalOptions) {
	f.globalOpts = opts
}

func (f *DefaultFactory) SetCliConfig(opts config.CLIConfig) {
	f.cliConfig = opts
}

func (f *DefaultFactory) IOStreams() *iostreams.IOStreams {
	return f.ioStreams
}

func (f *DefaultFactory) Survey() SurveyInterface {
	return f.survey
}

func (f *DefaultFactory) HTTPClient() *resty.Client {
	return f.httpClient
}

func (f *DefaultFactory) AssetClient() AssetInterface {
	return f.assetClient
}

func (f *DefaultFactory) ReleaseClient() ReleaseInterface {
	return f.releaseClient
}

func (f *DefaultFactory) MarketplaceClient() MarketplaceInterface {
	return f.marketplaceClient
}

func (f *DefaultFactory) Datastore() DatastoreInterface {
	return f.datastore
}

func (f *DefaultFactory) DeploymentClient() DeploymentInterface {
	return f.deploymentClient
}

func (f *DefaultFactory) ConfigFilePath() string {
	return f.globalOpts.ConfigFilePath
}

func (f *DefaultFactory) GlobalOptions() *config.GlobalOptions {
	return f.globalOpts
}

func (f *DefaultFactory) CliConfig() config.CLIConfig {
	return f.cliConfig
}

func (f *DefaultFactory) GraphQLURL() string {
	if f.globalOpts.GraphqlEndpoint != "" {
		return f.globalOpts.GraphqlEndpoint
	}
	return f.cliConfig.GraphqlEndpoint
}

func (f *DefaultFactory) Region() string {
	if f.globalOpts.Region != "" {
		return f.globalOpts.Region
	}
	return f.cliConfig.DefaultRegion
}

func (f *DefaultFactory) APIKey() string {
	if f.globalOpts.APIKey != "" {
		return f.globalOpts.APIKey
	}
	return f.cliConfig.APIKey
}

func (f *DefaultFactory) APISecret() string {
	if f.globalOpts.APISecret != "" {
		return f.globalOpts.APISecret
	}
	return f.cliConfig.APISecret
}

func (f *DefaultFactory) Timeout() time.Duration {
	return f.globalOpts.Timeout
}

func (f *DefaultFactory) Deadline() time.Time {
	return f.globalOpts.Deadline
}

func GetHTTPClient(apiKey, apiSecret string) *resty.Client {
	client := resty.New()
	client.SetBasicAuth(apiKey, apiSecret)
	client.SetHeader("X-Neru-ApiAccountId", apiKey)
	client.SetHeader("X-Neru-TraceId", uuid.New().String())
	return client
}

func getDatastore(graphQLURL string, httpClient *resty.Client) *api.Datastore {
	gqlClient := api.NewGraphQLClient(graphQLURL, httpClient)
	return api.NewDatastore(gqlClient)
}

func getWebsocketConnectionClient(apiKey, apiSecret string) *api.WebsocketConnectionClient {
	return api.NewWebsocketConnectionClient(apiKey, apiSecret)
}
