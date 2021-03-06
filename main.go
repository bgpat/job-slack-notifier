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

package main

import (
	"flag"
	"os"

	jsnv1beta1 "github.com/bgpat/job-slack-notifier/api/v1beta1"
	"github.com/bgpat/job-slack-notifier/controllers"
	"github.com/bgpat/job-slack-notifier/notifier"
	batchv1 "k8s.io/api/batch/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {

	jsnv1beta1.AddToScheme(scheme)
	corev1.AddToScheme(scheme)
	batchv1.AddToScheme(scheme)
	batchv1beta1.AddToScheme(scheme)
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var slackToken string
	var slackUsername string
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&slackToken, "slack-token", "", "The API token for slack bot.")
	flag.StringVar(&slackUsername, "slack-username", "job-slack-notifier", "The user name for the notification message.")
	flag.Parse()

	ctrl.SetLogger(zap.Logger(true))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		LeaderElection:     enableLeaderElection,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	jobNotifierReconciler := &controllers.JobNotifierReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("JobNotifier"),
	}
	err = jobNotifierReconciler.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "JobNotifier")
		os.Exit(1)
	}

	podReconciler := &controllers.PodReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Pod"),
	}
	err = podReconciler.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pod")
		os.Exit(1)
	}

	jobReconciler := &controllers.JobReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Job"),
	}
	err = jobReconciler.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Job")
		os.Exit(1)
	}

	cronJobReconciler := &controllers.CronJobReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("CronJob"),
	}
	err = cronJobReconciler.SetupWithManager(mgr)
	if err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "CronJob")
		os.Exit(1)
	}

	notifier.DefaultNotifier = notifier.NewNotifier(
		jobNotifierReconciler,
		podReconciler,
		jobReconciler,
		cronJobReconciler,
		slackToken,
		slackUsername,
	)

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
