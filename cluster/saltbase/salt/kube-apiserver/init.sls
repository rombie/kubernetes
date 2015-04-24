{% if grains.cloud is defined %}
{% if grains.cloud in ['aws', 'gce', 'vagrant'] %}
# TODO: generate and distribute tokens on other cloud providers.
/srv/kubernetes/known_tokens.csv:
  file.managed:
    - source: salt://kube-apiserver/known_tokens.csv
#    - watch_in:
#      - service: kube-apiserver
{% endif %}
{% endif %}

# Copy kube-apiserver manifest to manifests folder for kubelet.
/etc/kubernetes/manifests/kube-apiserver.manifest:
  file.managed:
    - source: salt://kube-apiserver/kube-apiserver.manifest
    - template: jinja
    - user: root
    - group: root
    - mode: 644
    - makedirs: true
    - dir_mode: 755

#stop legacy kube-apiserver service 
stop_kube-apiserver:
  service.dead:
    - name: kube-apiserver
    - enable: None
