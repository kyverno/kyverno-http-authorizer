package ctxprovider

import (
	"context"

	"github.com/kyverno/kyverno/pkg/clients/dclient"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ContextProvider struct {
	client dclient.Interface
}

func NewContextProvider(c dclient.Interface) *ContextProvider {
	return &ContextProvider{
		client: c,
	}
}

func (cp *ContextProvider) ListResources(apiVersion, resource, namespace string) (*unstructured.UnstructuredList, error) {
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	resourceInteface := cp.getResourceClient(groupVersion, resource, namespace)
	return resourceInteface.List(context.TODO(), metav1.ListOptions{})
}

func (cp *ContextProvider) GetResource(apiVersion, resource, namespace, name string) (*unstructured.Unstructured, error) {
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	resourceInteface := cp.getResourceClient(groupVersion, resource, namespace)
	return resourceInteface.Get(context.TODO(), name, metav1.GetOptions{})
}

func (cp *ContextProvider) PostResource(apiVersion, resource, namespace string, data map[string]any) (*unstructured.Unstructured, error) {
	groupVersion, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return nil, err
	}
	resourceInteface := cp.getResourceClient(groupVersion, resource, namespace)
	return resourceInteface.Create(context.TODO(), &unstructured.Unstructured{Object: data}, metav1.CreateOptions{})
}

func (cp *ContextProvider) getResourceClient(groupVersion schema.GroupVersion, resource string, namespace string) dynamic.ResourceInterface {
	client := cp.client.GetDynamicInterface().Resource(groupVersion.WithResource(resource))
	if namespace != "" {
		return client.Namespace(namespace)
	} else {
		return client
	}
}
