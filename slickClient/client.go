package slickClient

import (
	"crypto/tls"
	"fmt"
	"github.com/slickqa/slick/slickqa"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"log"
	"time"
)

type SlickAuth struct {
	slickClient *SlickClient
}

type SlickClient struct {
	GrpcUrl string
	Token string
	Agents slickqa.AgentsClient
	Links slickqa.LinksClient
	connection *grpc.ClientConn
	jwtToken string
	expires  time.Time
	headers  map[string]string
}

func CreateClient(grpcUrl string, token string) (*SlickClient, error) {
	s := &SlickClient{
		Token: token,
		GrpcUrl: grpcUrl,
	}
	log.Printf("Connecting to slick at:" + grpcUrl)
	conn, err := grpc.Dial(grpcUrl, grpc.WithPerRPCCredentials(SlickAuth{slickClient: s}),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{ServerName: grpcUrl, InsecureSkipVerify: true})))
	if err != nil {
		return nil, fmt.Errorf("grpc connection error %s", err)
	}
	s.connection = conn
	//defer conn.Close()
	s.Agents = slickqa.NewAgentsClient(conn)
	s.Links = slickqa.NewLinksClient(conn)
	return s, nil
}

func (s *SlickClient) Close() {
	if s.connection != nil {
		s.connection.Close()
	}
}

func (auth SlickAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if auth.slickClient.jwtToken == "" || time.Now().After(auth.slickClient.expires) {
		conn, err := grpc.Dial(auth.slickClient.GrpcUrl, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: auth.slickClient.GrpcUrl, InsecureSkipVerify: true})))
		defer conn.Close()
		if err != nil {
			return nil, err
		}
		client := slickqa.NewAuthClient(conn)
		resp, err := client.LoginWithToken(context.Background(), &slickqa.ApiTokenLoginRequest{Token: auth.slickClient.Token})
		if err != nil {
			return nil, err
		}
		log.Printf("Got new JwtToken: %s", resp.Token)
		auth.slickClient.jwtToken = resp.Token
		auth.slickClient.expires = time.Now().Add(time.Duration(10 * time.Minute))
		headers := make(map[string]string)
		headers["Authorization"] = "Bearer " + auth.slickClient.jwtToken
		auth.slickClient.headers = headers
	}
	return auth.slickClient.headers, nil
}

func (auth SlickAuth) RequireTransportSecurity() bool {
	return true
}

