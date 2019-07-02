package v1alpha3

import (
	xdsapi "github.com/envoyproxy/go-control-plane/envoy/api/v2"
	networking "istio.io/api/networking/v1alpha3"
)

func findMatches(listenerMatch *networking.EnvoyFilter_ListenerMatch, listeners []*xdsapi.Listener) []*xdsapi.Listener {
	var matches []*xdsapi.Listener

LISTENER_MATCH:
	for _, listener := range listeners {
		if listenerMatch.GetName() != "" && listenerMatch.GetName() != listener.GetName() {
			continue
		}

		socketAddress := listener.Address.GetSocketAddress()
		if (socketAddress == nil || socketAddress.GetPortValue() > 0) &&
			listenerMatch.GetPortNumber() != socketAddress.GetPortValue() {
			continue
		}

		if (socketAddress == nil || socketAddress.GetNamedPort() != "") &&
			listenerMatch.GetPortName() != socketAddress.GetNamedPort() {
			continue
		}

		filterChainMatchConditions := listenerMatch.GetFilterChain()
		if filterChainMatchConditions != nil {
			filterChains := listener.GetFilterChains()
			if len(filterChains) == 0 {
				continue
			}

			for _, chain := range filterChains {
				filterChainMatch := chain.GetFilterChainMatch()

				sniMatchCondition := filterChainMatchConditions.GetSni()
				if sniMatchCondition != "" {
					if filterChainMatch == nil {
						continue LISTENER_MATCH
					}

					serverNames := filterChainMatch.GetServerNames()
					if len(serverNames) == 0 {
						continue LISTENER_MATCH
					}

					for _, name := range serverNames {
						if sniMatchCondition != name {
							continue LISTENER_MATCH
						}
					}
				}

				transportProtocolMatchCondition := filterChainMatchConditions.GetTransportProtocol()
				if transportProtocolMatchCondition != "" {
					transportProtocol := filterChainMatch.GetTransportProtocol()
					if transportProtocol != transportProtocolMatchCondition {
						continue LISTENER_MATCH
					}
				}

				filterMatchConditions := filterChainMatchConditions.GetFilter()
				if filterMatchConditions != nil {
					filters := chain.GetFilters()
					if len(filters) == 0 {
						continue LISTENER_MATCH
					}

					for _, filter := range filters {
						if filter.GetName() != filterMatchConditions.GetName() {
							continue LISTENER_MATCH
						}
					}
				}
			}
		}

		matches = append(matches, listener)
	}

	return matches
}
