package types

// Clouds represents the content of the
// ~/.config/openstack/clouds.yaml file. The data looks like:
//
//    clouds:
//      openstack:
//        ...
//        ... this is the part we want to copy over to ~/.config/openstack/clouds.yaml
//        ...
//
type Clouds struct {
	Clouds map[string]map[string]interface{} `json:"clouds"`
}
