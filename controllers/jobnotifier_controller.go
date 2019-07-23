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
	"github.com/go-logr/logr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/api/v1beta1"
)

// JobNotifierReconciler reconciles a JobNotifier object
type JobNotifierReconciler struct {
	client.Client
	Log logr.Logger
}

// +kubebuilder:rbac:groups=jsn.k8s.bgpat.net,resources=jobnotifiers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=jsn.k8s.bgpat.net,resources=jobnotifiers/status,verbs=get;update;patch

func (r *JobNotifierReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	return ctrl.Result{}, nil
}

func (r *JobNotifierReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&jsnv1beta1.JobNotifier{}).
		Complete(r)
}
