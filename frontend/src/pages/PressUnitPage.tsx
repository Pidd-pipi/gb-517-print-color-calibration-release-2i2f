
import { EntityPage } from '../components/EntityPage';
import { ENTITY_CONFIGS } from '../types/status';
import { usePressUnitStore } from '../stores/press-unit';
export default function PressUnitPage() { return <EntityPage config={ENTITY_CONFIGS[0]} useStore={usePressUnitStore} />; }
