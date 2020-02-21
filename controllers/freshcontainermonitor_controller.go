/*
Copyright 2020 Flavio Castelli <fcastelli@suse.com>.

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
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flavio/fresh-container/pkg/fresh_container"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	"k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	infrastructurev1alpha1 "github.com/flavio/fresh-container-operator/api/v1alpha1"
)

const (
	ANNOTATION_ALLOW_UPDATE    = "fresh-container.autopilot"
	ANNOTATION_PREFIX          = "fresh-container.constraint/"
	ANNOTATION_NEXT_TAG_PREFIX = "fresh-container.nextTag/"
	ANNOTATION_LAST_CHECKED    = "fresh-container.lastChecked"
	LABEL_STALE                = "fresh-container.hasOutdatedContainers"
	DEFAULT_CHECK_INTERVAL     = 10
)

var (
	ErrorNoConstraints = errors.New("No constraints defined")
)

// FreshContainerMonitorReconciler reconciles a FreshContainerMonitor object
type FreshContainerMonitorReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=infrastructure.fresh-container-operator.suse.com,resources=freshcontainermonitors,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.fresh-container-operator.suse.com,resources=freshcontainermonitors/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps.v1,resources=deployments,verbs=get;list;watch;create;update;patch
func (r *FreshContainerMonitorReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	_ = r.Log.WithValues("freshcontainermonitor", req.NamespacedName)

	// Fetch the FreshContainerMonitor instance
	monitor := &infrastructurev1alpha1.FreshContainerMonitor{}

	err := r.Get(ctx, req.NamespacedName, monitor)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	deploymentList := &v1.DeploymentList{}

	err = r.List(ctx, deploymentList)
	if err != nil {
		r.Log.Error(err, "Cannot list deployments")
		return reconcile.Result{}, err
	}

	for _, d := range deploymentList.Items {
		//r.Log.WithValues(
		//  "name", d.Name,
		//  "namespace", d.Namespace,
		//).Info("Inspecting deployment")

		if r.hasBeenCheckedRecently(&d, monitor.Spec.CheckIntervalMinutes) {
			continue
		}

		containerUpgradeEvals, err := r.inspectDeploymentContainerImages(monitor.Spec.FreshContainerServerURL, &d)
		if err != nil && err != ErrorNoConstraints {
			r.Log.Error(
				err,
				"Something went wrong while analyzing the deployment",
				"deployment", d.Name,
				"namespace", d.Namespace,
			)
			continue
		}
		for containerName, eval := range containerUpgradeEvals {
			if eval.Stale {
				r.Log.WithValues(
					"deployment", d.Name,
					"namespace", d.Namespace,
					"container", containerName,
					"image", eval.Image,
					"constraint", eval.Constraint,
					"nextTag", eval.NextVersion,
				).Info("Deployment container can be updated")
			}
		}

		if err = r.modifyDeployment(ctx, &d, containerUpgradeEvals); err != nil {
			r.Log.Error(
				err,
				"Something went wrong while saving evaluation results into the deployment object",
				"deployment", d.Name,
				"namespace", d.Namespace,
			)
		}
	}

	return ctrl.Result{Requeue: true}, nil
}

func (r *FreshContainerMonitorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrastructurev1alpha1.FreshContainerMonitor{}).
		Complete(r)
}

type ContainerUpgradesEvals map[string]fresh_container.ImageUpgradeEvaluationResponse

func NewContainerUpgradesEvals() ContainerUpgradesEvals {
	return make(map[string]fresh_container.ImageUpgradeEvaluationResponse)
}

func (r *FreshContainerMonitorReconciler) inspectDeploymentContainerImages(server string, d *v1.Deployment) (ContainerUpgradesEvals, error) {
	client := fresh_container.NewClient(server)

	var constraints map[string]string
	constraints = make(map[string]string)

	evals := NewContainerUpgradesEvals()

	for annotation, value := range d.Spec.Template.Annotations {
		if strings.HasPrefix(annotation, ANNOTATION_PREFIX) {
			constraints[strings.TrimPrefix(annotation, ANNOTATION_PREFIX)] = value
		}
	}

	if len(constraints) == 0 {
		return evals, ErrorNoConstraints
	}

	for _, container := range d.Spec.Template.Spec.Containers {
		constraint, hasConstraint := constraints[container.Name]
		if !hasConstraint {
			continue
		}
		res, err := client.EvalUpgrade(container.Image, constraint)
		if err != nil {
			r.Log.WithValues(
				"image", container.Image,
				"constraint", constraint,
			).Error(err, "Constraint evaluation failed")
		}
		ready, err := res.IsReady()
		if err != nil {
			r.Log.WithValues(
				"image", container.Image,
				"constraint", constraint,
			).Error(err, "Constraint response: cannot determine status")
		}
		if ready {
			evals[container.Name] = res.Response
		}
	}

	return evals, nil
}

func (r *FreshContainerMonitorReconciler) modifyDeployment(ctx context.Context, d *v1.Deployment, evaluations ContainerUpgradesEvals) error {
	hasFreshContainers := false
	allowUpdate := false

	if len(evaluations) == 0 {
		return nil
	}

	_, hasAllowUpdateAttr := d.Annotations[ANNOTATION_ALLOW_UPDATE]
	if hasAllowUpdateAttr {
		var err error
		allowUpdate, err = strconv.ParseBool(d.Annotations[ANNOTATION_ALLOW_UPDATE])
		if err != nil {
			allowUpdate = false
		}
	}

	for containerName, eval := range evaluations {
		annotationNextTag := ANNOTATION_NEXT_TAG_PREFIX + containerName
		if eval.Stale {
			hasFreshContainers = true
			d.Annotations[annotationNextTag] = eval.NextVersion

			if allowUpdate {
				updateContainerImage(
					d,
					containerName,
					fmt.Sprintf("%s:%s", eval.Image, eval.NextVersion))

				r.Log.WithValues(
					"deployment", d.Name,
					"namespace", d.Namespace,
					"oldImage", fmt.Sprintf("%s:%s", eval.Image, eval.CurrentVersion),
					"newImage", fmt.Sprintf("%s:%s", eval.Image, eval.NextVersion),
					"constraint", eval.Constraint,
				).Info("Scheduling update of container image")
				r.Log.WithValues(
					"containers", d.Spec.Template.Spec.Containers).Info("TARGET DEPLOYMENT")
			}
		} else {
			delete(d.Annotations, annotationNextTag)
		}
	}

	if len(d.Labels) == 0 {
		d.Labels = make(map[string]string)
	}
	d.Labels[LABEL_STALE] = strconv.FormatBool(hasFreshContainers)

	d.Annotations[ANNOTATION_LAST_CHECKED] = time.Now().Format(time.RFC3339)

	return r.Update(ctx, d)
}

func updateContainerImage(d *v1.Deployment, containerName, image string) {
	for i, _ := range d.Spec.Template.Spec.Containers {
		if d.Spec.Template.Spec.Containers[i].Name == containerName {
			d.Spec.Template.Spec.Containers[i].Image = image
			return
		}
	}
}

func (r *FreshContainerMonitorReconciler) hasBeenCheckedRecently(d *v1.Deployment, checkInterval int) bool {
	if checkInterval == 0 {
		checkInterval = DEFAULT_CHECK_INTERVAL
	}

	lastChecked, checked := d.Annotations[ANNOTATION_LAST_CHECKED]
	if !checked {
		return false
	}
	lastCheckedTime, err := time.Parse(time.RFC3339, lastChecked)
	if err != nil {
		r.Log.Error(err, "Cannot parse 'last checked date'", "date", lastChecked)
		return false
	}

	nextCheck := lastCheckedTime.Add(time.Duration(checkInterval) * time.Minute)

	return time.Now().Before(nextCheck)
}
