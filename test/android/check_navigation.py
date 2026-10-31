#!/usr/bin/env python3
"""Check labelled bottom navigation in a UIAutomator dump from fictional Compose previews."""
import argparse
import math
import re
import xml.etree.ElementTree as ET


def names(node):
    """Accessible names of a node; Compose may join merged texts with ', '."""
    result = set()
    for value in (node.get('text', ''), node.get('content-desc', '')):
        if value:
            result.add(value)
            result.update(part for part in value.split(', ') if part)
    return result


def check(root, labels, selected, density):
    parents = {child: parent for parent in root.iter() for child in parent}

    def bounds(node):
        result = tuple(map(int, re.findall(r'-?\d+', node.get('bounds', ''))))
        assert len(result) == 4, node.attrib
        return result

    def item(node):
        # The item container is clickable; Android reports the already selected item
        # as selected but not clickable, because tapping it does nothing.
        while node.get('clickable') != 'true' and node.get('selected') != 'true' and node in parents:
            node = parents[node]
        return node if node.get('clickable') == 'true' or node.get('selected') == 'true' else None

    nav = []
    for label in labels:
        matches = [n for n in root.iter('node') if label in names(n)]
        assert matches, f'No accessible navigation name: {label}'
        items = [t for t in map(item, matches) if t is not None]
        assert items, f'Navigation is not an item: {label}'
        target = max(items, key=lambda n: bounds(n)[1])
        x1, y1, x2, y2 = bounds(target)
        minimum = math.ceil(48 * density / 160)
        assert x2 - x1 >= minimum and y2 - y1 >= minimum, f'Touch target below 48 dp: {label}'
        assert (target.get('selected') == 'true') == (label == selected), f'Wrong selected state: {label}'
        subtree = list(target.iter('node'))
        assert any(label in names({'text': n.get('text', '')}) for n in subtree), \
            f'Missing visible navigation label: {label}'
        # The visible label is the name; an icon description would make TalkBack read it twice.
        assert not any(label in names({'content-desc': n.get('content-desc', '')}) for n in subtree), \
            f'Navigation name read twice: {label}'
        nav.append(target)

    top = min(bounds(n)[1] for n in nav)
    height = max(bounds(n)[3] for n in root.iter('node'))
    assert top > height / 2, 'Navigation is not at the bottom'
    assert len({bounds(n) for n in nav}) == len(labels), 'Navigation items overlap'
    for n in root.iter('node'):
        if bounds(n)[1] >= top:
            text = n.get('text', '')
            # Destination labels and pending-count badges only.
            parts = [p for p in text.split(', ') if p] if text not in labels else [text]
            assert all(p in labels or (p.isdecimal() and len(p) <= 3) for p in parts), \
                f'Unexpected navigation text: {text}'
    assert any(n.get('text') == selected and bounds(n)[3] < top for n in root.iter('node')), \
        f'Visible page heading missing: {selected}'


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('xml')
    parser.add_argument('--labels', nargs='+', required=True)
    parser.add_argument('--selected', required=True)
    parser.add_argument('--density', type=int, required=True)
    args = parser.parse_args()
    check(ET.parse(args.xml).getroot(), args.labels, args.selected, args.density)
    print(f'PASS labelled navigation: {args.selected} ({len(args.labels)} labelled targets)')
