update roles
set permissions = array_remove(
    array_remove(
        array_remove(
            array_remove(
                array_remove(
                    array_remove(permissions, 'applications.read'),
                'applications.write'),
            'catalog.read'),
        'catalog.write'),
    'webhooks.read'),
'webhooks.write')
where name = 'owner';
