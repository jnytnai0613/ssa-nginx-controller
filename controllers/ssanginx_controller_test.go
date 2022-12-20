package controllers

import (
	"context"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	appsv1apply "k8s.io/client-go/applyconfigurations/apps/v1"
	corev1apply "k8s.io/client-go/applyconfigurations/core/v1"
	networkv1apply "k8s.io/client-go/applyconfigurations/networking/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ssanginxv1 "github.com/jnytnai0613/ssa-nginx-controller/api/v1"
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

func testSSANginx() *ssanginxv1.SSANginx {
	ssanginx := &ssanginxv1.SSANginx{}
	ssanginx.Namespace = constants.Namespace
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
	ssanginx.Spec.DeploymentSpec = (*ssanginxv1.DeploymentSpecApplyConfiguration)(depSpec)

	ssanginx.Spec.ServiceName = resouceName
	svcSpec := corev1apply.ServiceSpec()
	svcSpec.WithType(cip).
		WithPorts(corev1apply.ServicePort().
			WithProtocol(corev1.ProtocolTCP).
			WithPort(port).
			WithTargetPort(intstr.FromInt(port)))
	ssanginx.Spec.ServiceSpec = (*ssanginxv1.ServiceSpecApplyConfiguration)(svcSpec)

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
							WithName("nginx").
							WithPort(networkv1apply.ServiceBackendPort().
								WithNumber(port)))))))
	ssanginx.Spec.IngressSpec = (*ssanginxv1.IngressSpecApplyConfiguration)(ingressspec)
	ssanginx.Spec.IngressSecureEnabled = false
	return ssanginx
}

var err error

var _ = Describe("Test Controller", func() {
	ctx := context.Background()
	var stopFunc func()

	BeforeEach(func() {
		mgr, err := ctrl.NewManager(cfg, ctrl.Options{
			Scheme:             scheme,
			LeaderElection:     false,
			MetricsBindAddress: "0",
		})
		Expect(err).ShouldNot(HaveOccurred())

		reconciler := &SSANginxReconciler{
			Client:    mgr.GetClient(),
			Clientset: kclientset,
			Log:       ctrl.Log.WithName("controllers").WithName("NGINX"),
			Scheme:    scheme,
			Recorder:  mgr.GetEventRecorderFor("moco-controller"),
		}
		err = reconciler.SetupWithManager(mgr)
		Expect(err).ShouldNot(HaveOccurred())

		ctx, cancel := context.WithCancel(ctx)
		stopFunc = cancel
		go func() {
			err := mgr.Start(ctx)
			if err != nil {
				panic(err)
			}
		}()
		time.Sleep(100 * time.Millisecond)
	})

	AfterEach(func() {
		stopFunc()
		time.Sleep(100 * time.Millisecond)
	})

	It("should create custom resource", func() {
		cr := testSSANginx()
		err = kClient.Create(ctx, cr)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			cr := &ssanginxv1.SSANginx{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "test"}
			err := kClient.Get(ctx, key, cr)
			g.Expect(err).ShouldNot(HaveOccurred())
		}).Should(Succeed())
	})

	It("should create configmap resource", func() {
		cm := &corev1.ConfigMap{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 5*time.Second).Should(Succeed())

		Expect(cm.OwnerReferences).ShouldNot(BeEmpty())
		Expect(cm.Data["default.conf"]).Should(Equal(defaultconf))
		Expect(cm.Data["index.html"]).Should(Equal(indexhtml))
	})

	It("should create deployment resource", func() {
		dep := &appsv1.Deployment{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, dep)
			g.Expect(err).ShouldNot(HaveOccurred())
		}).Should(Succeed())

		var r int32 = rval
		Expect(dep.OwnerReferences).ShouldNot(BeEmpty())
		Expect(dep.Spec.Replicas).Should(Equal(&r))
		Expect(dep.Spec.Template.Spec.Containers[0].Name).Should(Equal(resouceName))
		Expect(dep.Spec.Template.Spec.Containers[0].Image).Should(Equal(image))
	})

	It("should create service resource", func() {
		svc := &corev1.Service{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, svc)
			g.Expect(err).ShouldNot(HaveOccurred())
		}).Should(Succeed())

		Expect(svc.OwnerReferences).ShouldNot(BeEmpty())
		Expect(svc.Spec.Type).Should(Equal(cip))
		Expect(svc.Spec.Ports[0].Protocol).Should(Equal(corev1.ProtocolTCP))
		Expect(svc.Spec.Ports[0].Port).Should(Equal(int32(port)))
		Expect(svc.Spec.Ports[0].TargetPort).Should(Equal(intstr.FromInt(port)))
	})

	It("should create ingress resource", func() {
		ing := &networkingv1.Ingress{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, ing)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 5*time.Second).Should(Succeed())

		bport := networkingv1.ServiceBackendPort{
			Number: int32(port),
		}
		Expect(ing.OwnerReferences).ShouldNot(BeEmpty())
		Expect(*ing.Spec.IngressClassName).Should(Equal(constants.IngressClassName))
		Expect(ing.Spec.Rules[0].Host).Should(Equal(hostname))
		Expect(ing.Spec.Rules[0].HTTP.Paths[0].Path).Should(Equal("/"))
		Expect(*ing.Spec.Rules[0].HTTP.Paths[0].PathType).Should(Equal(networkingv1.PathTypePrefix))
		Expect(ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name).Should(Equal("nginx"))
		Expect(ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port).Should(Equal(bport))
	})

	It("should create ca/server/client certtificate secret resource", func() {
		cr := &ssanginxv1.SSANginx{}
		key := client.ObjectKey{Namespace: constants.Namespace, Name: "test"}
		err := kClient.Get(ctx, key, cr)
		Expect(err).ShouldNot(HaveOccurred())

		cr.Spec.IngressSecureEnabled = true
		err = kClient.Update(ctx, cr)
		Expect(err).ShouldNot(HaveOccurred())

		caSec := &corev1.Secret{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "ca-secret"}
			err := kClient.Get(ctx, key, caSec)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 5*time.Second).Should(Succeed())

		cliSec := &corev1.Secret{}
		Eventually(func(g Gomega) {
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "cli-secret"}
			err := kClient.Get(ctx, key, cliSec)
			g.Expect(err).ShouldNot(HaveOccurred())
		}, 5*time.Second).Should(Succeed())

		Expect(caSec.OwnerReferences).ShouldNot(BeEmpty())
		Expect(cliSec.OwnerReferences).ShouldNot(BeEmpty())

		ing := &networkingv1.Ingress{}
		Eventually(func(g Gomega) {
			key = client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err = kClient.Get(ctx, key, ing)
			Expect(err).ShouldNot(HaveOccurred())
		}, 5*time.Second).Should(Succeed())

		Expect(ing.Spec.TLS[0].Hosts[0]).Should(Equal(hostname))
		Expect(ing.Spec.TLS[0].SecretName).Should(Equal(caSec.GetName()))
	})

	It("should update configmap name", func() {
		cr := &ssanginxv1.SSANginx{}
		key := client.ObjectKey{Namespace: constants.Namespace, Name: "test"}
		err := kClient.Get(ctx, key, cr)
		Expect(err).ShouldNot(HaveOccurred())

		cr.Spec.ConfigMapName = "nameupdate"
		err = kClient.Update(ctx, cr)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "nameupdate"}
			err := kClient.Get(ctx, key, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cm.GetName()).Should(Equal("nameupdate"))
		}).Should(Succeed())

		Eventually(func(g Gomega) {
			dep := &appsv1.Deployment{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, dep)
			g.Expect(err).ShouldNot(HaveOccurred())

			for i, _ := range dep.Spec.Template.Spec.Containers {
				s := strings.Split(dep.Spec.Template.Spec.Containers[i].Image, ":")
				if s[0] == constants.CompareImageName {
					if cr.Spec.ConfigMapName == dep.Spec.Template.Spec.Containers[i].VolumeMounts[0].Name {
						sameName := true
						g.Expect(sameName).Should(BeTrue())
					}
				}
			}

			for i, _ := range dep.Spec.Template.Spec.Volumes {
				if cr.Spec.ConfigMapName == dep.Spec.Template.Spec.Volumes[i].Name {
					sameName := true
					g.Expect(sameName).Should(BeTrue())
				}
			}
		}).Should(Succeed())
	})

	It("should update deployment name", func() {
		cr := &ssanginxv1.SSANginx{}
		key := client.ObjectKey{Namespace: constants.Namespace, Name: "test"}
		err := kClient.Get(ctx, key, cr)
		Expect(err).ShouldNot(HaveOccurred())

		cr.Spec.DeploymentName = "nameupdate"
		err = kClient.Update(ctx, cr)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			dep := &appsv1.Deployment{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "nameupdate"}
			err := kClient.Get(ctx, key, dep)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(dep.GetName()).Should(Equal("nameupdate"))
		}).Should(Succeed())
	})
})
