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
	currentAuth      SlickAuth
)

type SlickAuth struct {
	Token    string
	jwtToken string
	expires  time.Time
	headers  map[string]string
}

func (auth SlickAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if currentAuth.jwtToken == "" || time.Now().After(currentAuth.expires) {
		log.Printf("Url[0]: %s", uri[0])
		conn, err := grpc.Dial(slickGrpc, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: "slick.sofitest.com", InsecureSkipVerify: true})))
		if err != nil {
			return nil, err
		}
		client := slickqa.NewAuthClient(conn)
		resp, err := client.LoginWithToken(context.Background(), &slickqa.ApiTokenLoginRequest{Token: auth.Token})
		if err != nil {
			return nil, err
		}
		log.Printf("Got new JwtToken: %s", resp.Token)
		currentAuth.jwtToken = resp.Token
		currentAuth.expires = time.Now().Add(time.Duration(10 * time.Minute))
		headers := make(map[string]string)
		headers["Authorization"] = "Bearer " + currentAuth.jwtToken
		currentAuth.headers = headers
	}
	return currentAuth.headers, nil
}

func (auth SlickAuth) RequireTransportSecurity() bool {
	return true
}

func GetSlickClient(slickGrpcUrl string, token string) slickqa.AgentsClient {
	if slickAgentClient == nil {
		slickGrpc = slickGrpcUrl
		log.Printf("Connecting to slick at:" + slickGrpcUrl)
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
