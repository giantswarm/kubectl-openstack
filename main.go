package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	flag "github.com/spf13/pflag"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"sigs.k8s.io/yaml"

	"github.com/giantswarm/kubectl-openstack/internal/types"

	_ "k8s.io/client-go/plugin/pkg/client/auth"
	_ "k8s.io/client-go/plugin/pkg/client/auth/oidc"
)

const program = "kubectl-openstack"

func main() {
	ctx := context.Background()

	if err := mainE(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		os.Exit(1)
	}
}

func mainE(ctx context.Context) error {
	var cloudsFile string
	var defaultCloudsFile string
	if home := homedir.HomeDir(); home != "" {
		defaultCloudsFile = filepath.Join(home, ".config", "openstack", "clouds.yaml")
	}
	flag.StringVar(&cloudsFile, "clouds-file", defaultCloudsFile, "absolute path to the clouds.yaml file")

	var force bool
	flag.BoolVarP(&force, "force", "f", false, "force overwriting existing cloud (if it exists) in the clouds file")

	var kubeconfig string
	var defaultKubeconfig string
	if env := os.Getenv("KUBECONFIG"); env != "" {
		defaultKubeconfig = env
	} else if home := homedir.HomeDir(); home != "" {
		defaultKubeconfig = filepath.Join(home, ".kube", "config")
	}
	flag.StringVar(&kubeconfig, "kubeconfig", defaultKubeconfig, "absolute path to the kubeconfig file")

	var managementCluster string
	flag.StringVar(&managementCluster, "management-cluster", "", "(optional) name of the management cluster, if not set will be inferred from the API URL")

	var namespace string
	flag.StringVarP(&namespace, "namespace", "n", "", "(optional) namespace of the OpenstackCluster resource, required only if the cluster name is ambiguous")

	flag.Parse()

	if len(kubeconfig) == 0 {
		return fmt.Errorf("--kubeconfig flag / KUBECONFIG env var not set")
	}
	if len(cloudsFile) == 0 {
		return fmt.Errorf("--clouds-file flag not set")
	}

	if len(flag.Args()) != 2 || flag.Args()[0] != "login" {
		return fmt.Errorf("usage: %s login [-n NAMESPCE] OPENSTACKCLUSTER", program)
	}

	cluster := flag.Args()[1]

	clientset, dynamic, err := newClients(kubeconfig)
	if err != nil {
		return err
	}

	if len(managementCluster) == 0 {
		managementCluster, err = getManagementCluster(clientset)
		if err != nil {
			return err
		}
	}

	r, err := findOpenStackCluster(ctx, clientset, dynamic, namespace, cluster)
	if err != nil {
		return err
	}

	idRefKind, err := unstructuredGetString(r, "spec", "identityRef", "kind")
	if err != nil {
		return err
	}
	idRefName, err := unstructuredGetString(r, "spec", "identityRef", "name")
	if err != nil {
		return err
	}

	if idRefKind != "Secret" {
		return fmt.Errorf("only .spec.identityRef.kind = \"Secret\" supported but got %q", idRefKind)
	}

	secret, err := clientset.CoreV1().Secrets(r.GetNamespace()).Get(ctx, idRefName, v1.GetOptions{})
	if err != nil {
		return err
	}

	cloudsYaml, ok := secret.Data["clouds.yaml"]
	if !ok {
		return fmt.Errorf("secret/%s in %s does not have %q data field", secret.Name, secret.Namespace, "clouds.yaml")
	}

	secretClouds := types.Clouds{}
	err = yaml.Unmarshal(cloudsYaml, &secretClouds)
	if err != nil {
		return fmt.Errorf("unmarshaling YAML data from secret/%s in %s: %w", secret.Name, secret.Namespace, err)
	}

	if len(secretClouds.Clouds) == 0 {
		return fmt.Errorf("secret/%s in %s: expected single cloud data, got 0", secret.Name, secret.Namespace)
	}

	if len(secretClouds.Clouds) > 1 {
		ks := keys(secretClouds.Clouds)
		return fmt.Errorf("secret/%s in %s: expected single cloud data, got %d (%s)", secret.Name, secret.Namespace, len(ks), strings.Join(ks, ", "))
	}

	localClouds := types.Clouds{}
	cloudsFileData, err := os.ReadFile(cloudsFile)
	if err != nil {
		return err
	}
	err = yaml.Unmarshal(cloudsFileData, &localClouds)
	if err != nil {
		return fmt.Errorf("unmarshaling YAML data from %q: %w", cloudsFile, err)
	}

	cloudName := managementCluster + "-" + cluster

	localCloudsKeys := keys(localClouds.Clouds)
	if contains(localCloudsKeys, cloudName) {
		if force {
			fmt.Printf("Overwriting %q cloud in %s\n", cloudName, cloudsFile)
		} else {
			return fmt.Errorf("cloud %q already exists in %s, re-run with --force to overwrite\n", cloudName, cloudsFile)
		}
	} else {
		fmt.Printf("Writing %q cloud to %s\n", cloudName, cloudsFile)
	}

	localClouds.Clouds[cloudName] = secretClouds.Clouds[keys(secretClouds.Clouds)[0]]
	cloudsData, err := yaml.Marshal(localClouds)
	if err != nil {
		return fmt.Errorf("marshalling updated clouds YAML: %w", err)
	}

	err = os.WriteFile(cloudsFile, cloudsData, 0644)
	if err != nil {
		return err
	}

	fmt.Printf("\n")
	fmt.Printf("To use the cloud run:\n")
	fmt.Printf("\n")
	fmt.Printf("    openstack --os-cloud=%q server list", cloudName)
	fmt.Printf("\n")

	return nil
}

// TODO extract generic collection functions

func contains[S ~[]V, V comparable](s S, v V) bool {
	for _, vv := range s {
		if v == vv {
			return true
		}
	}

	return false
}

//func containsAny[S ~[]V, V comparable](s S, from S) (v V, found bool) {
//	for _, v := range from {
//		if contains(s, v) {
//			return v, true
//		}
//	}
//
//	return v, false
//}

func keys[M ~map[K]V, K comparable, V any](m M) []K {
	r := make([]K, 0, len(m))
	for k := range m {
		r = append(r, k)
	}
	return r
}

func newClients(kubeconfig string) (*kubernetes.Clientset, dynamic.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, nil, fmt.Errorf("building config from flags: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("creating client set from config: %w", err)
	}

	dynamic, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, nil, fmt.Errorf("creating dynamic client from config: %w", err)
	}

	return clientset, dynamic, nil
}

func findOpenStackCluster(ctx context.Context, clientset *kubernetes.Clientset, dynamic dynamic.Interface, namespace, name string) (*unstructured.Unstructured, error) {
	gvr, err := findOpenStackClusterGVR(clientset)
	if err != nil {
		return nil, err
	}

	if len(namespace) != 0 {
		r, err := dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, v1.GetOptions{})
		if err != nil {
			return nil, fmt.Errorf("from server: %w", err)
		}

		return r, nil
	}

	list, err := dynamic.Resource(gvr).Namespace("").List(ctx, v1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("from server: %w", err)
	}

	var r *unstructured.Unstructured
	count := 0
	for _, rr := range list.Items {
		if rr.GetName() == name {
			count++
			r = &rr
		}
	}

	if count == 0 {
		return nil, fmt.Errorf("cluster with name %q not found", name)
	}
	if count > 1 {
		return nil, fmt.Errorf("found more than one cluster with name %q, try re-running with --namespace flag", name)
	}

	return r, nil
}

func findOpenStackClusterGVR(clientset *kubernetes.Clientset) (schema.GroupVersionResource, error) {
	resource := "openstackclusters"
	group := "infrastructure.cluster.x-k8s.io"

	groups, err := clientset.DiscoveryClient.ServerGroups()
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("discovering server groups: %w", err)
	}

	for _, g := range groups.Groups {
		if strings.HasPrefix(g.PreferredVersion.GroupVersion, group) {
			gvr := schema.GroupVersionResource{
				Group:    group,
				Version:  g.PreferredVersion.Version,
				Resource: resource,
			}
			return gvr, nil
		}
	}

	return schema.GroupVersionResource{}, fmt.Errorf("GroupVersion %q is not registered with the cluster", group)
}

func unstructuredGetString(r *unstructured.Unstructured, path ...string) (string, error) {
	v, found, err := unstructured.NestedString(r.UnstructuredContent(), path...)
	if err != nil {
		return "", fmt.Errorf("%s %s/%s: %w", r.GetKind(), r.GetNamespace(), r.GetName(), err)
	}

	if !found {
		return "", fmt.Errorf("%s %s/%s path %q not found", r.GetKind(), r.GetNamespace(), r.GetName(), "."+strings.Join(path, "."))
	}

	if len(v) == 0 {
		return "", fmt.Errorf("%s %s/%s value for path %q is empty", r.GetKind(), r.GetNamespace(), r.GetName(), "."+strings.Join(path, "."))
	}

	return v, nil
}

func getManagementCluster(clientset *kubernetes.Clientset) (string, error) {
	url := clientset.RESTClient().Get().URL()
	hostname := url.Hostname()
	if !strings.HasPrefix(hostname, "api.") {
		return "", fmt.Errorf("cluster URL %q has unrecognized format, expected %q", url, "api.MANAGEMENT_CLUSTER...")
	}
	return strings.SplitN(hostname, ".", 3)[1], nil
}
