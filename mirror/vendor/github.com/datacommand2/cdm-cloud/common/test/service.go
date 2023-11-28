package test

import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/micro/go-micro/v2"
	"github.com/micro/go-micro/v2/auth"
	"github.com/micro/go-micro/v2/registry"
	"github.com/micro/go-micro/v2/server"
	"net"
	"time"
)

// Service 는 Registry 에 서비스를 등록하기 위한 구조체이다.
type Service struct {
	Name     string
	Address  string
	Metadata map[string]string
}

// Register 는 Registry 에 서비스를 등록하는 함수이다.
// 서비스 등록 후 전파될 때까지 일정시간이 소요될 수 있다.
func (s *Service) Register() {
	if s.Metadata == nil {
		s.Metadata = make(map[string]string)
	}

	srv := server.NewServer(
		server.Advertise(s.Address),
		server.Id(uuid.New().String()),
		server.Metadata(s.Metadata),
	)

	svc := micro.NewService(
		micro.Auth(auth.NewAuth()),
		micro.Server(srv),
		micro.Name(s.Name),
		micro.RegisterTTL(time.Minute),
	)

	go func() { _ = svc.Run() }()
}

// DiscoverService 는 Service Registry 에서 Service 를 찾는 함수이다.
func DiscoverService(name string) ([]Service, error) {
	// get registered service address
	r, err := registry.GetService(name)
	if err != nil && err != registry.ErrNotFound {
		return nil, err
	}

	var s []Service

	for _, v := range r {
		for _, n := range v.Nodes {
			s = append(s, Service{
				Name:     name,
				Address:  n.Address,
				Metadata: n.Metadata,
			})
		}
	}

	return s, nil
}

// LookupServiceAddress 는 로컬의 Resolver 를 통해 Service Host 의 IP 를 가져온다.
func LookupServiceAddress(host string, port int) ([]string, error) {
	ip, err := net.LookupIP(host)

	if err != nil {
		return nil, err

	} else if len(ip) == 0 {
		return nil, errors.New("could not found host")
	}

	var addr []string

	for _, i := range ip {
		addr = append(addr, fmt.Sprintf("%s:%d", i, port))
	}

	return addr, nil
}

// RegisterService 는 서비스를 찾아 registerName 으로 Registry 에 등록하는 함수이다.
func RegisterService(registerName, discoverName, host string, port int, metadata map[string]string) error {
	services, err := DiscoverService(discoverName)
	if err != nil {
		return err
	}

	// lookup service address from host
	if len(services) == 0 {
		addresses, err := LookupServiceAddress(host, port)
		if err != nil {
			return err
		}

		for _, address := range addresses {
			services = append(services, Service{
				Address:  address,
				Metadata: metadata,
			})
		}
	}

	// register service
	for _, service := range services {
		(&Service{
			Name:     registerName,
			Address:  service.Address,
			Metadata: service.Metadata,
		}).Register()
	}

	return nil
}
