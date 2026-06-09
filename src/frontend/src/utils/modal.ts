import { Modal as fallbackModal } from 'antd';

type ModalApi = Pick<typeof fallbackModal, 'confirm'>;

let activeModal: ModalApi = fallbackModal;

export function bindModal(instance: ModalApi | null): void {
  activeModal = instance ?? fallbackModal;
}

function getModal(): ModalApi {
  return activeModal;
}

export const modal: ModalApi = {
  confirm: (...args) => getModal().confirm(...args),
};
