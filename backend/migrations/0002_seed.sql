-- Single-tenant demo seed. Idempotent.

SET CLUSTER SETTING feature.vector_index.enabled = true;

USE stillmind;

INSERT INTO profiles (user_id, display_name, facts, goals, prefs)
VALUES (
  'demo-tenant:demo-user',
  'Friend',
  jsonb_build_object(
    'sleep_target', '22:45',
    'caffeine_cutoff', '14:00'
  ),
  jsonb_build_array('sleep','focus'),
  jsonb_build_object('session_length_min', 5, 'tone', 'calm')
)
ON CONFLICT (user_id) DO NOTHING;
