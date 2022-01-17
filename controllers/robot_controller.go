/*
Copyright 2022.
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

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types" // Required for Watching
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	robotsv1 "robolaunch.io/test-operator/api/v1"
)

// RobotReconciler reconciles a Robot object
type RobotReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=robots.robolaunch.io,resources=robots,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=robots.robolaunch.io,resources=robots/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=robots.robolaunch.io,resources=robots/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services/status,verbs=get
//+kubebuilder:rbac:groups=apps,resources=replicasets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=replicasets/status,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Robot object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.10.0/pkg/reconcile
func (r *RobotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	var robot robotsv1.Robot
	log.Info(req.Name)
	if err := r.Get(ctx, req.NamespacedName, &robot); err != nil {
		log.Error(err, "unable to fetch robot")
		log.Info(robot.Spec.Engin)
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	constructService := func(robot *robotsv1.Robot) (*corev1.Service, error) {
		service := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      robot.Name,
				Namespace: robot.Namespace,
				Labels:    robot.ObjectMeta.Labels,
			},
			Spec: *robot.Spec.ServiceTemplate.DeepCopy(),
		}
		if err := ctrl.SetControllerReference(robot, service, r.Scheme); err != nil {
			return nil, err
		}
		return service, nil
	}

	constructReplicas := func(robot *robotsv1.Robot) (*appsv1.ReplicaSet, error) {
		var repValue int32 = 1
		deploy := &appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      robot.Name,
				Namespace: robot.Namespace,

				Labels: robot.ObjectMeta.Labels,
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: &repValue,
				Selector: &metav1.LabelSelector{
					MatchLabels: robot.ObjectMeta.Labels,
				},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{
						Labels: robot.ObjectMeta.Labels,
					},
					Spec: *robot.Spec.PodTemplate.DeepCopy(),
				},
			},
		}
		log.Info(deploy.ObjectMeta.GetLabels()["app"])
		log.Info(deploy.Spec.Template.ObjectMeta.Labels["app"])

		if err := ctrl.SetControllerReference(robot, deploy, r.Scheme); err != nil {
			return nil, err
		}
		return deploy, nil
	}
	replica := &appsv1.ReplicaSet{}
	service := &corev1.Service{}

	err := r.Get(ctx, types.NamespacedName{Name: robot.Name, Namespace: robot.Namespace}, replica)

	if err != nil && errors.IsNotFound(err) {
		log.Info("Deployment not found, trigger deployment process")
		replica, err = constructReplicas(&robot)
		if err != nil {
			return ctrl.Result{}, err
		}
		log.Info("Creating Deployment", "deployment", replica.Name)

		if err := r.Create(ctx, replica); err != nil {
			log.Error(err, "error happend when replicaset")
			return ctrl.Result{}, err
		}
		robot.Status.ReplicaStatus = "Running"
	}

	err = r.Get(ctx, types.NamespacedName{Name: robot.Name, Namespace: robot.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		log.Info("Service not found, trigger deployment process")
		service, err = constructService(&robot)
		if err != nil {
			return ctrl.Result{}, err
		}

		if err := r.Create(ctx, service); err != nil {
			log.Error(err, "error happend when service creating")
			return ctrl.Result{}, err
		}
		log.V(1).Info("created service for Robot run")
		robot.Status.ServiceStatus = "Running"

	}

	if err := r.Status().Update(ctx, &robot); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RobotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&robotsv1.Robot{}).
		Owns(&appsv1.ReplicaSet{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
