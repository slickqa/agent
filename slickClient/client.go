package slickClient

import (
	"crypto/tls"
	"github.com/slickqa/slick/slickqa"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"log"
	"time"
)

var (
	slickAgentClient slickqa.AgentsClient
	slickGrpc        string
)

type SlickAuth struct {
	Token    string
	jwtToken string
	expires  time.Time
}

func (auth SlickAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if auth.jwtToken == "" || time.Now().After(auth.expires) {
		log.Printf("Url[0]: %s", uri[0])
		conn, err := grpc.Dial(slickGrpc, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{ServerName: "slick.sofitest.com", InsecureSkipVerify: true}))) //grpc.WithInsecure()) //WithTransportCredentials(credentials.NewTLS(nil)))
		if err != nil {
			return nil, err
		}
		client := slickqa.NewAuthClient(conn)
		resp, err := client.LoginWithToken(context.Background(), &slickqa.ApiTokenLoginRequest{Token: auth.Token})
		if err != nil {
			return nil, err
		}
		log.Printf("JwtToken: %s", resp.Token)
		auth.jwtToken = resp.Token
		auth.expires = time.Now().Add(time.Duration(10 * time.Minute))
	}
	headers := make(map[string]string)
	headers["Authorization"] = "Bearer " + auth.jwtToken
	return headers, nil
}

func (auth SlickAuth) RequireTransportSecurity() bool {
	return true
}

func GetSlickClient(slickGrpcUrl string, token string) slickqa.AgentsClient {
	if slickAgentClient == nil {
		slickGrpc = slickGrpcUrl
		log.Printf("Authenticating with slick for the first time.")
		//TODO: create and track my SlickAuth so it doesn't re-authenticate every time
		conn, err := grpc.Dial(slickGrpcUrl, grpc.WithPerRPCCredentials(SlickAuth{Token: token}),
			grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{ServerName: slickGrpcUrl, InsecureSkipVerify: true})))
		if err != nil {
			log.Printf("Error opening grpc connection %s", err)
		}
		//defer conn.Close()
		slickAgentClient = slickqa.NewAgentsClient(conn)
	}
	return slickAgentClient
}
