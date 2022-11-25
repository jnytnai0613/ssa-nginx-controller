package v1

import (
	"context"
	"errors"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	networkv1apply "k8s.io/client-go/applyconfigurations/networking/v1"

	"github.com/jnytnai0613/ssa-nginx-controller/pkg/constants"
)

const (
	cip                = corev1.ServiceTypeClusterIP
	defaultconf string = `server {
	listen 80 default_server;
	listen [::]:80 default_server ipv6only=on;
	root /usr/share/nginx/html;
	index index.html index.htm mod-index.html;
	server_name localhost;
}`
	indexhtml = `<!DOCTYPE html>
<html>
<head>
<title>Yeahhhhhhh!! Welcome to nginx!!</title>
<style>
html { color-scheme: light dark; }
body { width: 35em; margin: 0 auto;
font-family: Tahoma, Verdana, Arial, sans-serif; }
</style>
</head>
<body>
<h1>Yeahhhhhhh!! Welcome to nginx!!</h1>
<p>If you see this page, the nginx web server is successfully installed and
working. Further configuration is required.</p>
<p>For online documentation and support please refer to
<a href="http://nginx.org/">nginx.org</a>.<br/>
Commercial support is available at
<a href="http://nginx.com/">nginx.com</a>.</p>
<p><em>Thank you for using nginx.</em></p>
</body>
</html>`
	image             = "nginx"
	hostname          = "nginx.example.com"
	port              = 80
	resouceName       = "nginx"
	rval        int32 = 3
)

func HaveStatusErrorMessage(m types.GomegaMatcher) types.GomegaMatcher {
	return WithTransform(func(e error) (string, error) {
		statusErr := &apierrors.StatusError{}
		if !errors.As(e, &statusErr) {
			return "", fmt.Errorf("HaveStatusErrorMessage expects a *errors.StatusError, but got %T", e)
		}
		return statusErr.ErrStatus.Message, nil
	}, m)
}

func HaveStatusErrorReason(m types.GomegaMatcher) types.GomegaMatcher {
	return WithTransform(func(e error) (metav1.StatusReason, error) {
		statusErr := &apierrors.StatusError{}
		if !errors.As(e, &statusErr) {
			return "", fmt.Errorf("HaveStatusErrorReason expects a *errors.StatusError, but got %T", e)
		}
		return statusErr.ErrStatus.Reason, nil
	}, m)
}

func testSSANginx(svcName string, testport int32) *SSANginx {
	ssanginx := &SSANginx{}
	ssanginx.Namespace = "default"
	ssanginx.Name = "test"

	ssanginx.Spec.ConfigMapName = resouceName
	m := make(map[string]string)
	m["default.conf"] = defaultconf
	m["index.html"] = indexhtml
	ssanginx.Spec.ConfigMapData = m

	ssanginx.Spec.DeploymentName = resouceName
	depSpec := appsv1apply.DeploymentSpec()
	depSpec.WithReplicas(rval).
		WithTemplate(corev1apply.PodTemplateSpec().
			WithSpec(corev1apply.PodSpec().
				WithContainers(corev1apply.Container().
					WithName(resouceName).
					WithImage(image))))
	ssanginx.Spec.DeploymentSpec = (*DeploymentSpecApplyConfiguration)(depSpec)

	ssanginx.Spec.ServiceName = resouceName
	svcSpec := corev1apply.ServiceSpec()
	svcSpec.WithType(cip).
		WithPorts(corev1apply.ServicePort().
			WithProtocol(corev1.ProtocolTCP).
			WithPort(port).
			WithTargetPort(intstr.FromInt(int(port))))
	ssanginx.Spec.ServiceSpec = (*ServiceSpecApplyConfiguration)(svcSpec)

	ssanginx.Spec.IngressName = resouceName
	ingressspec := networkv1apply.IngressSpec()
	ingressspec.WithIngressClassName(constants.IngressClassName).
		WithRules(networkv1apply.IngressRule().
			WithHost(hostname).
			WithHTTP(networkv1apply.HTTPIngressRuleValue().
				WithPaths(networkv1apply.HTTPIngressPath().
					WithPath("/").
					WithPathType(networkingv1.PathTypePrefix).
					WithBackend(networkv1apply.IngressBackend().
						WithService(networkv1apply.IngressServiceBackend().
							WithName(svcName).
							WithPort(networkv1apply.ServiceBackendPort().
								WithNumber(testport)))))))
	ssanginx.Spec.IngressSpec = (*IngressSpecApplyConfiguration)(ingressspec)
	ssanginx.Spec.IngressSecureEnabled = false

	return ssanginx
}

var _ = Describe("Webhook Table Test", func() {
	DescribeTable("Validator Test", func(svcName string, testport int32, reason metav1.StatusReason, message string) {
		ssanginx := testSSANginx(svcName, testport)
		ctx := context.Background()
		err := k8sClient.Create(ctx, ssanginx)

		Expect(err).Should(HaveStatusErrorReason(Equal(reason)))
		Expect(err.Error()).Should(ContainSubstring(message))
	},
		Entry("service name for ingress does not match.", "test", int32(80), metav1.StatusReasonInvalid, "Must match service name."),
		Entry("service port for ingress does not match.", resouceName, int32(81), metav1.StatusReasonInvalid, "Must match service port number."),
	)

})
