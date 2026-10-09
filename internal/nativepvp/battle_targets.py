"""Read-only skill target observations, also embedded in the Python 2 client hotfix."""


def skill_target_hooks(process_command, skill_result, report):
    # Keep this function self-contained and Python 2 compatible for hotfix embedding.
    def process(self, user):
        previous = getattr(self, '_revival_skill_cast', None)
        context = None
        try:
            name, args = user.command
            if name in ('use_skill', 'use_support_skill', 'use_extra_support_skill',
                        'charm_use_skill', 'taunt_use_skill'):
                self._revival_cast_sequence = getattr(self, '_revival_cast_sequence', 0) + 1
                context = {'cast_id': self._revival_cast_sequence, 'eid': str(user.eid),
                           'skill_id': args[0], 'target_eids': [], 'target_roles': {}}
        except Exception:
            pass
        self._revival_skill_cast = context
        try:
            result = process_command(self, user)
        finally:
            self._revival_skill_cast = previous
        if context is not None and not context.get('incomplete'):
            try:
                report(self, 'skill_targets', context)
            except Exception:
                pass
        return result

    def resolve(self, effect_struct, effect_play, result_struct):
        context = getattr(self, '_revival_skill_cast', None)
        target_id = role = None
        try:
            # The native result is created after range/random/target confirmation.
            # Exclude counterattacks and passive skills executed inside this cast.
            if (context is not None and str(result_struct.user.eid) == context['eid']
                    and result_struct.skill is not None
                    and result_struct.skill.skill_id == context['skill_id']):
                target_id = str(result_struct.target.eid)
                role = int(result_struct.target.get_role_id())
        except Exception:
            if context is not None:
                context['incomplete'] = True
        result = skill_result(self, effect_struct, effect_play, result_struct)
        if target_id is not None and target_id not in context['target_eids']:
            context['target_eids'].append(target_id)
            context['target_roles'][target_id] = role
        return result

    return process, resolve
