package v1alpha3

import (
	"reflect"
	"testing"

	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/core"
	"github.com/envoyproxy/go-control-plane/envoy/api/v2/listener"
	networking "istio.io/api/networking/v1alpha3"
)

func TestFindMatches(t *testing.T) {
	seedListeners := []*xdsapi.Listener{
		{
			Address: core.Address{
				Address: &core.Address_Pipe{
					Pipe: &core.Pipe{
						Path: "example-path",
					},
				},
			},
			FilterChains: []listener.FilterChain{
				{
					Filters: []listener.Filter{
						{
							Name: "envoy.http_connection_manager",
						},
					},
					FilterChainMatch: &listener.FilterChainMatch{
						ServerNames:       []string{"www.*.com"},
						TransportProtocol: "tls",
					},
				},
			},
		},
		{
			Name: "nameListener",
			Address: core.Address{
				Address: &core.Address_Pipe{
					Pipe: &core.Pipe{
						Path: "some-path",
					},
				},
			},
		},
		{
			Name: "portNumberListener",
			Address: core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						PortSpecifier: &core.SocketAddress_PortValue{
							PortValue: uint32(433),
						},
					},
				},
			},
		},
		{
			Name: "portNameListener",
			Address: core.Address{
				Address: &core.Address_SocketAddress{
					SocketAddress: &core.SocketAddress{
						PortSpecifier: &core.SocketAddress_NamedPort{
							NamedPort: "some-named-port",
						},
					},
				},
			},
		},
		{
			Name: "filterChainSniListener",
			FilterChains: []listener.FilterChain{
				{
					FilterChainMatch: &listener.FilterChainMatch{
						ServerNames: []string{"www.example.com"},
					},
				},
			},
		},
		{
			Name: "filterChainTransportProtocolListener",
			FilterChains: []listener.FilterChain{
				{
					FilterChainMatch: &listener.FilterChainMatch{
						TransportProtocol: "raw_buffer",
					},
				},
			},
		},
		{
			Name: "filterChainFilterNameListener",
			FilterChains: []listener.FilterChain{
				{
					Filters: []listener.Filter{
						{
							Name: "envoy.ratelimit",
						},
					},
					FilterChainMatch: &listener.FilterChainMatch{
						ServerNames: []string{"www.acme.com"},
					},
				},
			},
		},
	}

	testCases := []struct {
		name              string
		matchConditions   *networking.EnvoyFilter_ListenerMatch
		existingListeners []*xdsapi.Listener
		result            []*xdsapi.Listener
	}{
		{
			name: "listeners matched by name",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				Name: "nameListener",
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "nameListener",
					Address: core.Address{
						Address: &core.Address_Pipe{
							Pipe: &core.Pipe{
								Path: "some-path",
							},
						},
					},
				},
			},
		},
		{
			name: "listeners matched by port number",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				PortNumber: 433,
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "portNumberListener",
					Address: core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: uint32(433),
								},
							},
						},
					},
				},
			},
		},
		{
			name: "listeners matched by port name",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				PortName: "some-named-port",
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "portNameListener",
					Address: core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								PortSpecifier: &core.SocketAddress_NamedPort{
									NamedPort: "some-named-port",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "listeners matched by filter chain sni",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
					Sni: "www.example.com",
				},
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "filterChainSniListener",
					FilterChains: []listener.FilterChain{
						{
							FilterChainMatch: &listener.FilterChainMatch{
								ServerNames: []string{"www.example.com"},
							},
						},
					},
				},
			},
		},
		{
			name: "listeners matched by filter chain transport protocol",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
					TransportProtocol: "raw_buffer",
				},
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "filterChainTransportProtocolListener",
					FilterChains: []listener.FilterChain{
						{
							FilterChainMatch: &listener.FilterChainMatch{
								// TODO: add validation for the two accepted transportprotocol values
								TransportProtocol: "raw_buffer",
							},
						},
					},
				},
			},
		},
		{
			name: "listeners matched by filter name in filter chain",
			matchConditions: &networking.EnvoyFilter_ListenerMatch{
				FilterChain: &networking.EnvoyFilter_ListenerMatch_FilterChainMatch{
					Filter: &networking.EnvoyFilter_ListenerMatch_FilterMatch{
						Name: "envoy.ratelimit",
					},
				},
			},
			existingListeners: seedListeners,
			result: []*xdsapi.Listener{
				{
					Name: "filterChainFilterNameListener",
					FilterChains: []listener.FilterChain{
						{
							Filters: []listener.Filter{
								{
									Name: "envoy.ratelimit",
								},
							},
							FilterChainMatch: &listener.FilterChainMatch{
								ServerNames: []string{"www.acme.com"},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		ret := findMatches(tc.matchConditions, tc.existingListeners)
		if !reflect.DeepEqual(tc.result, ret) {
			t.Errorf("test case:  %s; expecting %v but got %v", tc.name, tc.result, ret)
		}
	}
}
