/*
Copyright 2019 bgpat.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package jobnotifier

import (
	"context"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/pkg/apis/jsn/v1beta1"
	//appsv1 "k8s.io/api/apps/v1"
	//batchv1 "k8s.io/api/batch/v1"
	//batchv1beta1 "k8s.io/api/batch/v1beta1"
	//corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/apimachinery/pkg/runtime/schema"
	//"k8s.io/apimachinery/pkg/types"
	//"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	//"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

var log = logf.Log.WithName("controller")

// Add creates a new JobNotifier Controller and adds it to the Manager with default RBAC. The Manager will set fields on the Controller
// and Start it when the Manager is Started.
func Add(mgr manager.Manager) error {
	return add(mgr, newReconciler(mgr))
}

// newReconciler returns a new reconcile.Reconciler
func newReconciler(mgr manager.Manager) reconcile.Reconciler {
	return &ReconcileJobNotifier{Client: mgr.GetClient(), scheme: mgr.GetScheme()}
}

// add adds a new Controller to mgr with r as the reconcile.Reconciler
func add(mgr manager.Manager, r reconcile.Reconciler) error {
	// Create a new controller
	c, err := controller.New("jobnotifier-controller", mgr, controller.Options{Reconciler: r})
	if err != nil {
		return err
	}

	// Watch for changes to JobNotifier
	err = c.Watch(&source.Kind{Type: &jsnv1beta1.JobNotifier{}}, &handler.EnqueueRequestForObject{})
	if err != nil {
		return err
	}

	mgr.Add(manager.RunnableFunc(func(stopCh <-chan struct{}) (err error) {
		config := mgr.GetConfig()
		k8sClient, err = kubernetes.NewForConfig(config)
		if err != nil {
			log.Error(err, "Failed to initialize config")
			return err
		}
		<-stopCh
		watchersMu.Lock()
		defer watchersMu.Unlock()
		for _, w := range watchers {
			w.stop()
		}
		return nil
	}))
	return nil
}

var _ reconcile.Reconciler = &ReconcileJobNotifier{}

// ReconcileJobNotifier reconciles a JobNotifier object
type ReconcileJobNotifier struct {
	client.Client
	scheme *runtime.Scheme
}

// Reconcile reads that state of the cluster for a JobNotifier object and makes changes based on the state read
// and what is in the JobNotifier.Spec
// Automatically generate RBAC rules to allow the Controller to read and write Deployments
// +kubebuilder:rbac:groups=,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=,resources=pods/status,verbs=get
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;update
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// +kubebuilder:rbac:groups=jsn.k8s.bgpat.net,resources=jobnotifiers,verbs=get;list;watch
// +kubebuilder:rbac:groups=jsn.k8s.bgpat.net,resources=jobnotifiers/status,verbs=get;update;patch
func (r *ReconcileJobNotifier) Reconcile(request reconcile.Request) (reconcile.Result, error) {
	// Fetch the JobNotifier instance
	instance := &jsnv1beta1.JobNotifier{}
	err := r.Get(context.TODO(), request.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return reconcile.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return reconcile.Result{}, err
	}
	log.Info(
		"notifier",
		"namespace", instance.Namespace,
		"name", instance.Name,
		"selector", instance.Spec.Selector,
	)
	return reconcile.Result{}, watchJob(instance)
}
