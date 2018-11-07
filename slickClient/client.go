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
	Token    string
	GrpcUrl  string
	jwtToken string
	expires  time.Time
	headers  map[string]string

}

type SlickClient struct {
	Token string
	Agents slickqa.AgentsClient
	connection *grpc.ClientConn
}

func CreateClient(grpcUrl string, token string) (*SlickClient, error) {
	s := &SlickClient{
		Token: token,
	}
	log.Printf("Connecting to slick at:" + grpcUrl)
	conn, err := grpc.Dial(grpcUrl, grpc.WithPerRPCCredentials(SlickAuth{Token: token, GrpcUrl: grpcUrl}),
		grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{ServerName: grpcUrl, InsecureSkipVerify: true})))
	if err != nil {
		return nil, fmt.Errorf("grpc connection error %s", err)
	}
	s.connection = conn
	//defer conn.Close()
	s.Agents = slickqa.NewAgentsClient(conn)
	return s, nil
}

func (s *SlickClient) Close() {
	if s.connection != nil {
		s.connection.Close()
	}
}

func (auth SlickAuth) GetRequestMetadata(ctx context.Context, uri ...string) (map[string]string, error) {
	if auth.jwtToken == "" || time.Now().After(auth.expires) {
		conn, err := grpc.Dial(auth.GrpcUrl, grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			ServerName: auth.GrpcUrl, InsecureSkipVerify: true})))
		defer conn.Close()
		if err != nil {
			return nil, err
		}
		client := slickqa.NewAuthClient(conn)
		resp, err := client.LoginWithToken(context.Background(), &slickqa.ApiTokenLoginRequest{Token: auth.Token})
		if err != nil {
			return nil, err
		}
		log.Printf("Got new JwtToken: %s", resp.Token)
		auth.jwtToken = resp.Token
		auth.expires = time.Now().Add(time.Duration(10 * time.Minute))
		headers := make(map[string]string)
		headers["Authorization"] = "Bearer " + auth.jwtToken
		auth.headers = headers
	}
	return auth.headers, nil
}

func (auth SlickAuth) RequireTransportSecurity() bool {
	return true
}

