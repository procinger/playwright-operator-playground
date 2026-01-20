docker_prune_settings(
  disable = False,
  max_age_mins = 30,
  num_builds = 2,
  interval_hrs = 0,
  keep_recent = 2
)

docker_build(
  ref = 'localhost:5001/operator/api',
  context = 'operator/api',
  dockerfile = 'operator/api/Dockerfile',
)

docker_build(
  ref = 'localhost:5001/operator/dashboard',
  context = 'operator/dashboard',
  dockerfile = 'operator/dashboard/Dockerfile',
)

k8s_yaml('./manifest/operator.yaml')
k8s_resource(
  workload = 'operator',
  labels = ['Operator'],
  port_forwards = ['8080:3000'],
)

k8s_yaml('./manifest/pvc.yaml')
k8s_resource(
  objects=[
    'playwright-results:persistentvolumeclaim',
  ],
  new_name='Volume',
  labels = ['Operator'],
)

k8s_yaml('./manifest/rbac.yaml')
k8s_resource(
  objects=[
    'operator:serviceaccount',
    'operator:role',
    'operator:rolebinding',
  ],
  new_name='RBAC',
  labels = ['Operator'],
)
