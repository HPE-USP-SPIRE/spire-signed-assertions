package api

import (
	"context"
	// "errors"
	"flag"
	"fmt"

	"github.com/mitchellh/cli"
	"github.com/spiffe/go-spiffe/v2/proto/spiffe/workload"
	common_cli "github.com/spiffe/spire/pkg/common/cli"
	
)

func NewFetchJWTCommand() cli.Command {
	return newFetchJWTCommand(common_cli.DefaultEnv, newWorkloadClient)
}

func newFetchJWTCommand(env *common_cli.Env, clientMaker workloadClientMaker) cli.Command {
	return adaptCommand(env, clientMaker, new(fetchJWTCommand))
}

type fetchJWTCommand struct {
	audience common_cli.CommaStringsFlag
	spiffeID string
}

func (c *fetchJWTCommand) name() string {
	return "fetch svidng"
}

func (c *fetchJWTCommand) synopsis() string {
	return "Fetches a SVID-NG from the Workload API"
}

func (c *fetchJWTCommand) run(ctx context.Context, env *common_cli.Env, client *workloadClient) error {

	svidResp, err := c.fetchJWTSVID(ctx, client)
	if err != nil {
		return err
	}

	for _, svid := range svidResp.Svids {
		fmt.Printf("token(%s):\n\t%s\n", svid.SpiffeId, svid.Svid)
	}

	return nil
}

func (c *fetchJWTCommand) appendFlags(fs *flag.FlagSet) {
	fs.Var(&c.audience, "audience", "Append the ID to a given one.")
	fs.StringVar(&c.spiffeID, "spiffeID", "", "SPIFFE ID subject (optional)")
}

func (c *fetchJWTCommand) fetchJWTSVID(ctx context.Context, client *workloadClient) (*workload.JWTSVIDResponse, error) {
	ctx, cancel := client.prepareContext(ctx)
	defer cancel()
	return client.FetchJWTSVID(ctx, &workload.JWTSVIDRequest{
		Audience: c.audience,
		SpiffeId: c.spiffeID,
	})
}

func (c *fetchJWTCommand) fetchJWTBundles(ctx context.Context, client *workloadClient) (*workload.JWTBundlesResponse, error) {
	ctx, cancel := client.prepareContext(ctx)
	defer cancel()
	stream, err := client.FetchJWTBundles(ctx, &workload.JWTBundlesRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to receive SVID-NG bundles: %w", err)
	}
	return stream.Recv()
}
