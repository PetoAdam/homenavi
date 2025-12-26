import React from 'react';

export default class BaseNodeEditor extends React.PureComponent {
  get selectedNode() {
    return this.props.selectedNode || null;
  }

  updateSelectedNode = (mapNode) => {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;

    const { applyEditorUpdate } = this.props;
    applyEditorUpdate((prev) => ({
      ...prev,
      nodes: (Array.isArray(prev.nodes) ? prev.nodes : []).map((n) => {
        if (!n || n.id !== selectedNode.id) return n;
        return mapNode(n);
      }),
    }));
  };

  updateSelectedNodeBatched = (batchKey, mapNode) => {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;

    const { applyEditorUpdateBatched } = this.props;
    applyEditorUpdateBatched(batchKey, (prev) => ({
      ...prev,
      nodes: (Array.isArray(prev.nodes) ? prev.nodes : []).map((n) => {
        if (!n || n.id !== selectedNode.id) return n;
        return mapNode(n);
      }),
    }));
  };

  setSelectedNodeKind = (kind) => {
    const selectedNode = this.selectedNode;
    if (!selectedNode) return;

    const { defaultNodeData } = this.props;
    this.updateSelectedNode((n) => ({
      ...n,
      kind,
      data: defaultNodeData(kind),
    }));
  };

  setSelectedNodeData = (patch) => {
    this.updateSelectedNodeBatched('props', (n) => ({
      ...n,
      data: { ...(n.data || {}), ...(patch || {}) },
    }));
  };

  setSelectedNodeUI = (patch) => {
    this.updateSelectedNodeBatched('props', (n) => ({
      ...n,
      data: {
        ...(n.data || {}),
        ui: { ...(n.data?.ui || {}), ...(patch || {}) },
      },
    }));
  };
}
