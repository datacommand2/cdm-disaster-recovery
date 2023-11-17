package kubernetes

import (
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"context"
	"fmt"
	"os"
)

// GetClientSet Kubernetes 의 client set
func GetClientSet() (*kubernetes.Clientset, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	return kubernetes.NewForConfig(config)
}

// GetKubePodByHostName 현재 '서비스'가 실행되고 있는 Pod 의 정보 조회
func GetKubePodByHostName(ctx context.Context, client *kubernetes.Clientset, namespace string) (*v1.Pod, error) {
	return client.CoreV1().Pods(namespace).Get(ctx, os.Getenv("HOSTNAME"), metav1.GetOptions{})
}

// GetPodListByKubernetes 현재 실행되고 있는 Pod list 조회
func GetPodListByKubernetes(ctx context.Context, client *kubernetes.Clientset, namespace string) (*v1.PodList, error) {
	return client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
}

// GetPodListByLabelSelector 특정 서비스이름의 모든 pod list 조회
func GetPodListByLabelSelector(ctx context.Context, client *kubernetes.Clientset, namespace, serviceName string) (*v1.PodList, error) {
	return client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("name=%s", serviceName),
	})
}
