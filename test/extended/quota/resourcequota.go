package quota

import (
	"context"
	"fmt"
	"time"

	g "github.com/onsi/ginkgo/v2"
	o "github.com/onsi/gomega"

	imagev1 "github.com/openshift/api/image/v1"
	exutil "github.com/openshift/origin/test/extended/util"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilwait "k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
)

var _ = g.Describe("[sig-api-machinery][Feature:ResourceQuota]", func() {
	defer g.GinkgoRecover()
	oc := exutil.NewCLI("object-count-rq")

	g.Describe("Object count", func() {
		g.It(fmt.Sprintf("should properly count the number of imagestreams resources [apigroup:image.openshift.io]"), func() {
			clusterAdminKubeClient := oc.AdminKubeClient()
			clusterAdminImageClient := oc.AdminImageClient().ImageV1()
			testProject := oc.SetupProject()
			testResourceQuotaName := "count-imagestreams"

			rq := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{Name: testResourceQuotaName, Namespace: testProject},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"openshift.io/imagestreams": resource.MustParse("10"),
					},
				},
			}

			_, err := clusterAdminKubeClient.CoreV1().ResourceQuotas(testProject).Create(context.Background(), rq, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				if !equality.Semantic.DeepEqual(actualResourceQuota.Spec.Hard, actualResourceQuota.Status.Hard) {
					return fmt.Errorf("%#v != %#v", actualResourceQuota.Spec.Hard, actualResourceQuota.Status.Hard)
				}
				expectedUsedStatus := corev1.ResourceList{
					"openshift.io/imagestreams": resource.MustParse("0"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actul: %#v, expected: %#v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})

			g.By("creating an image stream and checking the usage")
			imageStream := &imagev1.ImageStream{
				ObjectMeta: metav1.ObjectMeta{Name: "empty-is"},
			}
			_, err = clusterAdminImageClient.ImageStreams(testProject).Create(context.Background(), imageStream, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"openshift.io/imagestreams": resource.MustParse("1"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %#v, expected: %#v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("deleting the image stream and checking the usage")
			err = clusterAdminImageClient.ImageStreams(testProject).Delete(context.Background(), imageStream.Name, metav1.DeleteOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"openshift.io/imagestreams": resource.MustParse("0"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %v, expected: %v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		g.It("should properly count the number of persistentvolumeclaims resources [Serial]", func() {
			testProject := oc.SetupProject()
			testResourceQuotaName := "my-resource-quota-" + testProject
			pvcName := "myclaim-" + testProject
			clusterAdminKubeClient := oc.AdminKubeClient()

			rq := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{Name: testResourceQuotaName, Namespace: testProject},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"persistentvolumeclaims": resource.MustParse("1"),
					},
				},
			}

			pvc := &corev1.PersistentVolumeClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name: pvcName,
				},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse("3Gi"),
						},
					},
				},
			}

			g.By("create the persistent volume and checking the usage")
			_, err := clusterAdminKubeClient.CoreV1().ResourceQuotas(testProject).Create(context.Background(), rq, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"persistentvolumeclaims": resource.MustParse("0"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %#v, expected: %#v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = clusterAdminKubeClient.CoreV1().PersistentVolumeClaims(testProject).Create(context.Background(), pvc, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"persistentvolumeclaims": resource.MustParse("1"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %v, expected: %v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			_, err = clusterAdminKubeClient.CoreV1().PersistentVolumeClaims(testProject).Create(context.Background(), pvc, metav1.CreateOptions{})
			o.Expect(err).To(o.HaveOccurred())
			o.Expect(err.Error()).To(o.MatchRegexp(pvcName + `.*forbidden.*[Ee]xceeded quota`))

			g.By("deleting the persistent volume and checking the usage")
			err = clusterAdminKubeClient.CoreV1().PersistentVolumeClaims(testProject).Delete(context.Background(), pvcName, metav1.DeleteOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())
			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"persistentvolumeclaims": resource.MustParse("0"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %v, expected: %v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		})

		g.It("check the quota after import-image with --all option [Skipped:Disconnected]", func() {
			testProject := oc.SetupProject()
			testResourceQuotaName := "my-imagestream-quota-" + testProject
			clusterAdminKubeClient := oc.AdminKubeClient()

			rq := &corev1.ResourceQuota{
				ObjectMeta: metav1.ObjectMeta{Name: testResourceQuotaName, Namespace: testProject},
				Spec: corev1.ResourceQuotaSpec{
					Hard: corev1.ResourceList{
						"openshift.io/imagestreams": resource.MustParse("10"),
					},
				},
			}

			g.By("create the imagestreams and checking the usage")
			_, err := clusterAdminKubeClient.CoreV1().ResourceQuotas(testProject).Create(context.Background(), rq, metav1.CreateOptions{})
			o.Expect(err).NotTo(o.HaveOccurred())

			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"openshift.io/imagestreams": resource.MustParse("0"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %#v, expected: %#v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("trying to tag a container image")
			err = oc.AsAdmin().WithoutNamespace().Run("import-image").Args("centos", "--from=quay.io/openshifttest/alpine", "--confirm=true", "--all=true", "-n", testProject).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = oc.AsAdmin().WithoutNamespace().Run("tag").Args("quay.io/openshifttest/base-alpine@sha256:3126e4eed4a3ebd8bf972b2453fa838200988ee07c01b2251e3ea47e4b1f245c", "--source=docker", "mystream:latest", "-n", testProject).Execute()
			o.Expect(err).NotTo(o.HaveOccurred())

			err = exutil.WaitForAnImageStreamTag(oc, testProject, "mystream", "latest")
			o.Expect(err).NotTo(o.HaveOccurred())

			g.By("checking the imagestream usage again")
			err = waitForResourceQuotaStatus(clusterAdminKubeClient, testResourceQuotaName, testProject, func(actualResourceQuota *corev1.ResourceQuota) error {
				expectedUsedStatus := corev1.ResourceList{
					"openshift.io/imagestreams": resource.MustParse("2"),
				}
				if !equality.Semantic.DeepEqual(actualResourceQuota.Status.Used, expectedUsedStatus) {
					return fmt.Errorf("unexpected current total usage: actual: %#v, expected: %#v", actualResourceQuota.Status.Used, expectedUsedStatus)
				}
				return nil
			})
			o.Expect(err).NotTo(o.HaveOccurred())
		})
	})
})

func waitForResourceQuotaStatus(clusterAdminKubeClient kubernetes.Interface, name string, namespace string, conditionFn func(*corev1.ResourceQuota) error) error {
	var pollErr error
	err := utilwait.PollImmediate(100*time.Millisecond, QuotaWaitTimeout, func() (done bool, err error) {
		quota, err := clusterAdminKubeClient.CoreV1().ResourceQuotas(namespace).Get(context.Background(), name, metav1.GetOptions{})
		if err != nil {
			pollErr = err
			return false, nil
		}
		err = conditionFn(quota)
		if err == nil {
			return true, nil
		}
		pollErr = err
		return false, nil
	})
	if err != nil {
		err = fmt.Errorf("%s: %s", err, pollErr)
	}
	return err
}
