package k8s

import (
	"strconv"
	"strings"

	networking "k8s.io/api/networking/v1"
)

type Ingress struct {
	Manifest     networking.Ingress `json:"manifest"`
	Name         string             `json:"name"`
	Namespace    string             `json:"namespace"`
	IngressClass string             `json:"ingressClass"`
	Plugins      []string           `json:"plugins"`
	Paths        []Path             `json:"paths"`
	CreationTime string             `json:"creationTime"`
	LoadBalancer string             `json:"loadBalancer"`
}

type Path struct {
	Host        string `json:"host"`
	Path        string `json:"path"`
	PathType    string `json:"pathType"`
	ServiceName string `json:"serviceName"`
	ServicePort string `json:"servicePort"`
}

func mapIngresses(items []networking.Ingress) []Ingress {
	out := make([]Ingress, 0, len(items))
	for _, item := range items {
		// Items in an IngressList can omit their individual type metadata.
		item.APIVersion, item.Kind = networking.SchemeGroupVersion.String(), "Ingress"
		ing := Ingress{
			Manifest: item,
			Name:     item.Name, Namespace: item.Namespace, IngressClass: "N/A",
			Plugins: []string{}, Paths: []Path{}, CreationTime: item.CreationTimestamp.String(),
		}
		if item.Spec.IngressClassName != nil {
			ing.IngressClass = *item.Spec.IngressClassName
		} else if class := item.Annotations["kubernetes.io/ingress.class"]; class != "" {
			ing.IngressClass = class
		}
		for plugin := range strings.SplitSeq(item.Annotations["konghq.com/plugins"], ",") {
			if p := strings.TrimSpace(plugin); p != "" {
				ing.Plugins = append(ing.Plugins, p)
			}
		}
		for _, rule := range item.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			host := rule.Host
			if host == "" {
				host = "*"
			}
			for _, rulePath := range rule.HTTP.Paths {
				path := Path{Host: host, Path: rulePath.Path, PathType: "N/A", ServiceName: "N/A", ServicePort: "N/A"}
				if rulePath.PathType != nil {
					path.PathType = string(*rulePath.PathType)
				}
				if svc := rulePath.Backend.Service; svc != nil {
					path.ServiceName = svc.Name
					path.ServicePort = svc.Port.Name
					if path.ServicePort == "" {
						path.ServicePort = strconv.Itoa(int(svc.Port.Number))
					}
				}
				ing.Paths = append(ing.Paths, path)
			}
		}
		if addresses := item.Status.LoadBalancer.Ingress; len(addresses) > 0 {
			ing.LoadBalancer = addresses[0].IP
			if ing.LoadBalancer == "" {
				ing.LoadBalancer = addresses[0].Hostname
			}
		}
		out = append(out, ing)
	}
	return out
}
