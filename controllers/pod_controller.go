/*

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

package controllers

import (
	"context"

	"github.com/bgpat/job-slack-notifier/notifier"
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodReconciler reconciles a Pod object
type PodReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

func (r *PodReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	var pod corev1.Pod
	if err := r.Get(ctx, req.NamespacedName, &pod); err != nil {
		r.Log.Info("not found pod", "pod", req.NamespacedName, "error", err)
		return ctrl.Result{}, nil
	}
	if owner := metav1.GetControllerOf(&pod); owner != nil {
		go notifier.NotifyJob(client.ObjectKey{
			Namespace: pod.Namespace,
			Name:      owner.Name,
		})
	}
	return ctrl.Result{}, nil
}

func (r *PodReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Pod{}).
		Complete(r)
}
