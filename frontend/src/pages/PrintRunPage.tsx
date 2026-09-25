
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { usePrintRunStore } from '../stores/print-run';
export default function PrintRunPage() { return <EntityPage config={ENTITY_CONFIGS[1]} useStore={usePrintRunStore} />; }
