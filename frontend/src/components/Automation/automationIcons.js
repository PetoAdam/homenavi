import {
  faBolt,
  faClock,
  faCodeBranch,
  faEnvelope,
  faHandPointer,
  faMoon,
  faPaperPlane,
  faRepeat,
} from '@fortawesome/free-solid-svg-icons';

export function iconForNodeKind(kind) {
  const k = String(kind || '').trim().toLowerCase();
  switch (k) {
    case 'trigger.manual':
      return faHandPointer;
    case 'trigger.schedule':
      return faClock;
    case 'trigger.device_state':
      return faBolt;
    case 'action.send_command':
      return faPaperPlane;
    case 'action.notify_email':
      return faEnvelope;
    case 'logic.sleep':
      return faMoon;
    case 'logic.if':
      return faCodeBranch;
    case 'logic.for':
      return faRepeat;
    default:
      return null;
  }
}
