package testutil

import (
	"testing"
	"time"

	"vcr-cli/pkg/config"

	"github.com/cli/cli/v2/pkg/iostreams"
	"github.com/golang/mock/gomock"

	"vcr-cli/pkg/cmdutil"
	"vcr-cli/testutil/mocks"
)

const (
	DefaultAPIKey         = "abcd1234"
	DefaultAPISecret      = "Te5ts3cret"
	DefaultRegion         = "eu-west-1"
	DefaultTimeout        = 10 * time.Minute
	DefaultTraceID        = "3e020425-499f-44ba-ad71-4d29510af169"
	DefaultGraphQL        = "https://api.vonage.com/graphql"
	DefaultConfigFilePath = "testdata/config.yaml"
)

var (
	DefaultDeadline      = time.Now().Add(DefaultTimeout)
	DefaultHTTPClient    = cmdutil.GetHTTPClient(DefaultAPIKey, DefaultAPISecret).SetHeader("X-Neru-Trace-Id", DefaultTraceID)
	DefaultGlobalOptions = config.GlobalOptions{
		ConfigFilePath:  DefaultConfigFilePath,
		APISecret:       DefaultAPISecret,
		APIKey:          DefaultAPIKey,
		Region:          DefaultRegion,
		Timeout:         DefaultTimeout,
		Deadline:        DefaultDeadline,
		GraphqlEndpoint: DefaultGraphQL,
	}
	DefaultCredentials = config.Credentials{
		APIKey:    DefaultAPIKey,
		APISecret: DefaultAPISecret,
	}
	DefaultCliConfig = config.CLIConfig{
		GraphqlEndpoint: DefaultGraphQL,
		DefaultRegion:   DefaultRegion,
		Credentials:     DefaultCredentials,
	}
)

// DefaultFactoryMock returns a mock of the Factory interface with default values.
func DefaultFactoryMock(t *testing.T, io *iostreams.IOStreams, da cmdutil.AssetInterface, dr cmdutil.ReleaseInterface, ds cmdutil.DatastoreInterface, dc cmdutil.DeploymentInterface, su cmdutil.SurveyInterface) cmdutil.Factory {
	f := mocks.NewMockFactory(gomock.NewController(t))
	f.EXPECT().Survey().Return(su).AnyTimes()
	f.EXPECT().IOStreams().Return(io).AnyTimes()
	f.EXPECT().HTTPClient().Return(DefaultHTTPClient).AnyTimes()
	f.EXPECT().AssetClient().Return(da).AnyTimes()
	f.EXPECT().ReleaseClient().Return(dr).AnyTimes()
	f.EXPECT().Datastore().Return(ds).AnyTimes()
	f.EXPECT().DeploymentClient().Return(dc).AnyTimes()
	f.EXPECT().Region().Return(DefaultRegion).AnyTimes()
	f.EXPECT().APIKey().Return(DefaultAPIKey).AnyTimes()
	f.EXPECT().APISecret().Return(DefaultAPISecret).AnyTimes()
	f.EXPECT().Deadline().Return(DefaultDeadline).AnyTimes()
	f.EXPECT().GraphQLURL().Return(DefaultGraphQL).AnyTimes()
	f.EXPECT().Timeout().Return(DefaultTimeout).AnyTimes()
	f.EXPECT().ConfigFilePath().Return(DefaultConfigFilePath).AnyTimes()
	f.EXPECT().GlobalOptions().Return(&DefaultGlobalOptions).AnyTimes()
	f.EXPECT().CliConfig().Return(DefaultCliConfig).AnyTimes()
	f.EXPECT().SetCliConfig(gomock.Any()).AnyTimes()
	f.EXPECT().InitDatastore(gomock.Any(), gomock.Any()).AnyTimes()
	f.EXPECT().InitDeploymentClient(gomock.Any(), gomock.Any()).AnyTimes()
	return f
}
