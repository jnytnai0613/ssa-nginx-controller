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
	resouceName        = "nginx"
	rval        int32  = 3
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
	cip   = corev1.ServiceTypeClusterIP
	image = "nginx"
	tport = 80
)

func testSSANginx() *ssanginxv1.SSANginx {
	ssanginx := &ssanginxv1.SSANginx{}
	ssanginx.Namespace = constants.Namespace
	ssanginx.Name = "test"

	ssanginx.Spec.DeploymentName = resouceName
	depSpec := appsv1apply.DeploymentSpec()
	depSpec.WithReplicas(rval)
	depSpec.WithTemplate(corev1apply.PodTemplateSpec().
		WithSpec(corev1apply.PodSpec().
			WithContainers(corev1apply.Container().
				WithName(resouceName).
				WithImage(image))))
	ssanginx.Spec.DeploymentSpec = (*ssanginxv1.DeploymentSpecApplyConfiguration)(depSpec)

	ssanginx.Spec.ConfigMapName = resouceName
	m := make(map[string]string)
	m["default.conf"] = defaultconf
	m["index.html"] = indexhtml
	ssanginx.Spec.ConfigMapData = m

	ssanginx.Spec.ServiceName = resouceName
	svcSpec := corev1apply.ServiceSpec()
	svcSpec.WithType(cip)
	svcSpec.WithPorts(corev1apply.ServicePort().
		WithProtocol(corev1.ProtocolTCP).
		WithPort(80).
		WithTargetPort(intstr.FromInt(tport)))
	ssanginx.Spec.ServiceSpec = (*ssanginxv1.ServiceSpecApplyConfiguration)(svcSpec)

	ssanginx.Spec.IngressName = resouceName
	ingressspec := networkv1apply.IngressSpec()
	ingressspec.WithRules(networkv1apply.IngressRule().
		WithHost("nginx.example.com").
		WithHTTP(networkv1apply.HTTPIngressRuleValue().
			WithPaths(networkv1apply.HTTPIngressPath().
				WithPath("/").
				WithPathType("Prefix").
				WithBackend(networkv1apply.IngressBackend().
					WithService(networkv1apply.IngressServiceBackend().
						WithName("nginx").
						WithPort(networkv1apply.ServiceBackendPort().
							WithNumber(80)))))))
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
		Eventually(func(g Gomega) {
			cm := &corev1.ConfigMap{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cm.OwnerReferences).ShouldNot(BeEmpty())
		}, "5s").Should(Succeed())
	})

	It("should create deployment resource", func() {
		Eventually(func(g Gomega) {
			dep := &appsv1.Deployment{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, dep)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(dep.OwnerReferences).ShouldNot(BeEmpty())
		}).Should(Succeed())
	})

	It("should create service resource", func() {
		Eventually(func(g Gomega) {
			svc := &corev1.Service{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, svc)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(svc.OwnerReferences).ShouldNot(BeEmpty())
		}).Should(Succeed())
	})

	It("should create ingress resource", func() {
		Eventually(func(g Gomega) {
			ing := &networkingv1.Ingress{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: resouceName}
			err := kClient.Get(ctx, key, ing)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(ing.OwnerReferences).ShouldNot(BeEmpty())
		}, "5s").Should(Succeed())
	})

	It("should create ca/server/client certtificate secret resource", func() {
		cr := &ssanginxv1.SSANginx{}
		key := client.ObjectKey{Namespace: constants.Namespace, Name: "test"}
		err := kClient.Get(ctx, key, cr)
		Expect(err).ShouldNot(HaveOccurred())

		cr.Spec.IngressSecureEnabled = true
		err = kClient.Update(ctx, cr)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			sec := &corev1.Secret{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "ca-secret"}
			err := kClient.Get(ctx, key, sec)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(sec.OwnerReferences).ShouldNot(BeEmpty())
		}, "5s").Should(Succeed())

		Eventually(func(g Gomega) {
			sec := &corev1.Secret{}
			key := client.ObjectKey{Namespace: constants.Namespace, Name: "cli-secret"}
			err := kClient.Get(ctx, key, sec)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(sec.OwnerReferences).ShouldNot(BeEmpty())
		}, "5s").Should(Succeed())
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
