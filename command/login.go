package command

import (
	"fmt"
	"strings"

	"github.com/hashicorp/cap/util"
	"github.com/hashicorp/nomad/api"
	"github.com/hashicorp/nomad/lib/oidc"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/posener/complete"
)

type LoginCommand struct {
	Meta
}

func (c *LoginCommand) Help() string {
	helpText := `
Usage: nomad login

  Does stuff...

General Options:

  ` + generalOptionsUsage(usageOptsDefault|usageOptsNoNamespace)

	return helpText
}

func (c *LoginCommand) AutocompleteFlags() complete.Flags {
	return complete.Flags{}
}

func (c *LoginCommand) AutocompleteArgs() complete.Predictor {
	return complete.PredictNothing
}

func (c *LoginCommand) Synopsis() string {
	return "Login to Nomad using OIDC"
}

func (c *LoginCommand) Name() string { return "license get" }

func (c *LoginCommand) Run(args []string) int {

	// ===========================
	//   GET DEFAULT AUTH METHOD
	// ===========================

	// respList, err := c.project.Client().ListOIDCAuthMethods(c.Ctx, &empty.Empty{})

	client, err := c.Meta.Client()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error initializing client: %s", err))
		return 1
	}

	methods, _, err := client.AuthMethods().List(nil)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error gettng auth methods: %s", err))
		return 1
	}

	if len(methods) == 0 {
		c.Ui.Error("Must configure an auth method to use login command")
		return 1
	}

	// TODO: search for a "default" method
	amName := methods[0].Name

	// =========================
	//   START CALLBACK SERVER
	// =========================

	// Start our callback server
	callbackSrv, err := oidc.NewCallbackServer()
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	// ====================
	//   GET THE AUTH URL
	// ====================

	nonce := callbackSrv.Nonce()

	getAuthArgs := api.AuthUrlArgs{
		AuthMethod:  amName,
		RedirectUri: callbackSrv.RedirectUri(),
		ClientNonce: nonce,
	}

	urlResp, _, err := client.OIDC().GetAuthUrl(&getAuthArgs, nil)
	if err != nil {
		fmt.Println("ERROR")
		c.Ui.Error(err.Error())
		return 1
	}

	url := urlResp.URL

	// ============================
	//   OPEN AUTH URL IN BROWSER
	// ============================

	// Open the auth URL in the user browser or ask them to visit it.
	// We purposely use fmt here and NOT c.ui because the ui will truncate
	// our URL (a known bug).
	fmt.Printf(strings.TrimSpace(outVisitURL)+"\n\n", url)
	if err := util.OpenURL(url); err != nil {
		fmt.Println("error opening auth url", "err", err)
	}

	// ============================
	//   WAIT FOR RESP ON CHANNEL
	// ============================

	// Wait
	var req *structs.OIDCCallbackRequest
	select {
	// case c.Ctx.Done()
	// 	fmt.Println("User go bye bye")
	// 	return 1

	case err := <-callbackSrv.ErrorCh():
		fmt.Println(err)
		return 1

	case req = <-callbackSrv.SuccessCh():
		// We got our data!
	}

	// ============================
	//   COMPLETE THE AUTH
	// ============================

	// Complete the auth
	req.Auth.AuthMethod = amName
	cbArgs := api.CallbackArgs{
		AuthMethod:  amName,
		RedirectUri: callbackSrv.RedirectUri(),
		ClientNonce: nonce,
		Code:        req.Auth.Code,
		State:       req.Auth.State,
	}

	token, _, err := client.OIDC().Callback(&cbArgs, nil)
	if err != nil {
		fmt.Println(err)
		return 1
	}

	c.Ui.Output(
		"You have successfully logged in. Paste the following command into your shell to use the token.",
	)

	c.Ui.Output(
		fmt.Sprintf("export NOMAD_TOKEN=%s", *token),
	)

	return 0
}

const (
	outVisitURL = `
Complete the authentication by visiting your authentication provider.
Opening your browser window now. If the browser window does not launch,
please visit the URL below:

%s
`
)
