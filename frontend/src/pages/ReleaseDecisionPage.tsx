
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { useReleaseDecisionStore } from '../stores/release-decision';
export default function ReleaseDecisionPage() { return <EntityPage config={ENTITY_CONFIGS[3]} useStore={useReleaseDecisionStore} />; }
