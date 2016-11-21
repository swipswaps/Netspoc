#!/usr/bin/perl

use strict;
use warnings;
use Test::More;
use Test::Differences;
use lib 't';
use Test_Netspoc;

my ($title, $in, $out);

############################################################
$title = 'Unexptected attribute at bridged interface';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside;  no_in_acl; dhcp_server; routing = OSPF; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Invalid attributes 'dhcp_server', 'no_in_acl', 'routing' for bridged interface at line 7 of STDIN
END

test_err($title, $in, $out);

############################################################
$title = 'Bridged network must not have NAT';
############################################################

$in = <<'END';
network:n1/left = {
 ip = 10.1.1.0/24;
 nat:x = { ip = 10.1.2.0/26; dynamic; }
}

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Only identity NAT allowed for bridged network:n1/left
END

test_err($title, $in, $out);

############################################################
$title = 'Bridged network must not inherit NAT';
############################################################

$in = <<'END';
any:a = { link = network:n1/left; nat:x = { ip = 10.1.2.0/26; dynamic; } }
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Must not inherit nat:x at bridged network:n1/left from any:[network:n1/left]
END

test_err($title, $in, $out);

############################################################
$title = 'Bridged network must not have hosts';
############################################################

$in = <<'END';
network:n1/left = {
 ip = 10.1.1.0/24;
 host:h = { ip = 10.1.1.10; }
}

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Bridged network:n1/left must not have host definition (not implemented)
END

test_err($title, $in, $out);

############################################################
$title = 'Other network must not use prefix name of bridged networks';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }

router:r1 = {
 interface:n1/right;
 interface:n1;
}
network:n1 = { ip = 10.2.2.0/24; }
END

$out = <<'END';
Error: Must not define network:n1 together with bridged networks of same name
END

test_err($title, $in, $out);

############################################################
$title = 'Bridged networks must use identical IP addresses';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.2.2.0/24; }
END

$out = <<'END';
Error: network:n1/left and network:n1/right must have identical ip/mask
END

test_err($title, $in, $out);

############################################################
$title = 'Missing layer 3 interface';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Must define interface:bridge.n1 for corresponding bridge interfaces
END

test_err($title, $in, $out);

############################################################
$title = 'Layer 3 interface must not have secondary IP';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1, 10.1.1.2; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: Layer3 interface:bridge.n1 must not have secondary interface:bridge.n1.2
END

test_err($title, $in, $out);

############################################################
$title = 'Layer 3 IP must match bridged network IP/mask';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.2.2.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: interface:bridge.n1's IP doesn't match IP/mask of bridged networks
END

test_err($title, $in, $out);

############################################################
$title = 'Brdged networks must be connected by bridge';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:r1 = {
 model = ASA;
 managed;
 interface:n1/left = { ip = 10.1.1.1; hardware = inside; }
 interface:n1/right = { ip = 10.1.1.2; hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: network:n1/right and network:n1/left must be connected by bridge
END

test_err($title, $in, $out);

############################################################
$title = 'Bridge must connect at least two networks';
############################################################

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge1 = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
}
router:bridge2 = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Error: router:bridge1 can't bridge a single network
END

test_err($title, $in, $out);

############################################################
$title = 'Bridged must not be used solitary';
############################################################

$in = <<'END';
network:n1/right = { ip = 10.1.1.0/24; }
END

$out = <<'END';
Warning: Bridged network:n1/right must not be used solitary
END

test_warn($title, $in, $out);

############################################################
$title = 'Bridged network must not be unnumbered';
############################################################

$in = <<'END';
network:n1/left = { unnumbered; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { unnumbered; hardware = device; }
 interface:n1/left = { hardware = inside; }
 interface:n1/right = { hardware = outside; }
}

network:n1/right = { unnumbered; }
END

$out = <<'END';
Error: Unnumbered network:n1/left must not have attribute 'bridged'
Error: Layer3 interface:bridge.n1 must not be unnumbered
Error: Unnumbered network:n1/right must not have attribute 'bridged'
Error: interface:bridge.n1/left must not be linked to unnumbered network:n1/left
Error: interface:bridge.n1/right must not be linked to unnumbered network:n1/right
END

test_err($title, $in, $out);

############################################################
$title = 'Admin access to bridge';
############################################################

my $topology = <<'END';

network:intern = { 
 ip = 10.1.1.0/24;
 host:netspoc = { ip = 10.1.1.111; }
}

router:asa = {
 model = IOS;
 #managed;
 interface:intern = {
  ip = 10.1.1.101; 
  hardware = Ethernet0;
 }
 interface:dmz/left = { 
  ip = 192.168.0.101; 
  hardware = Ethernet1;
 }
}

network:dmz/left = { ip = 192.168.0.0/24; }

router:bridge = {
 model = ASA;
 managed;
 policy_distribution_point = host:netspoc;
 interface:dmz = { ip = 192.168.0.9; hardware = device; }
 interface:dmz/left = { hardware = inside; }
 interface:dmz/right = { hardware = outside; }
}

network:dmz/right = { ip = 192.168.0.0/24;}

router:extern = { 
 interface:dmz/right = { ip = 192.168.0.1; }
 interface:extern;
}

network:extern = { ip = 10.9.9.0/24; }
END

$in = $topology . <<'END';
service:admin = {
 user =  interface:bridge.dmz;
 permit src = network:intern; dst = user; prt = tcp 22; 
}
END

$out = <<'END';
--bridge
! [ IP = 192.168.0.9 ]
END

test_run($title, $in, $out);

############################################################
$title = 'Admin access to bridge auto interface';
############################################################
$in = $topology . <<'END';
service:admin = {
 user =  interface:bridge.[auto];
 permit src = network:intern; dst = user; prt = tcp 22; 
}
END

# $out is unchanged
test_run($title, $in, $out);

############################################################
$title = 'Admin access to bridge all interfaces';
############################################################
$in = $topology . <<'END';
service:admin = {
 user =  interface:bridge.[all];
 permit src = network:intern; dst = user; prt = tcp 22; 
}
END

# $out is unchanged
test_run($title, $in, $out);

############################################################
$title = 'Access to both sides of bridged network';
############################################################

$topology =~ s/policy_distribution_point = .*;//;
$topology =~ s/#managed/managed/;
$in = $topology . <<'END';
service:test = {
 user = network:dmz/left, network:dmz/right;
 permit src = user; dst = host:[network:intern]; prt = tcp 80; 
}
END

$out = <<'END';
--bridge
access-list outside_in extended permit tcp 192.168.0.0 255.255.255.0 host 10.1.1.111 eq 80
access-list outside_in extended deny ip any any
access-group outside_in in interface outside
END

test_run($title, $in, $out);

############################################################
$title = 'Access through bridged ASA';
############################################################

$in = $topology . <<'END';
service:test = {
 user = network:extern;
 permit src = user; dst = host:[network:intern]; prt = tcp 80; 
}
END

$out = <<'END';
--bridge
access-list outside_in extended permit tcp 10.9.9.0 255.255.255.0 host 10.1.1.111 eq 80
access-list outside_in extended deny ip any any
access-group outside_in in interface outside
END

test_run($title, $in, $out);

############################################################
$title = 'Duplicate auto interface';
############################################################

# Two auto interfaces are found in topology,
# but are combined into a single layer 3 interface.

$in = <<'END';
network:n1/left = { ip = 10.1.1.0/24; }

router:bridge = {
 model = ASA;
 managed;
 interface:n1 = { ip = 10.1.1.1; hardware = device; }
 interface:n1/left  = { hardware = left; }
 interface:n1/right = { hardware = right; }
}
network:n1/right = { ip = 10.1.1.0/24; }

router:r1 = {
 managed;
 model = ASA;
 interface:n1/left = { ip = 10.1.1.3; hardware = n1; }
 interface:n2 = { ip = 10.1.2.3; hardware = n2; }
}

router:r2 = {
 managed;
 model = ASA;
 interface:n1/right = { ip = 10.1.1.2; hardware = n1; }
 interface:n2 = { ip = 10.1.2.2; hardware = n2; }
}

network:n2 = { ip = 10.1.2.0/24; }

service:s = {
 user = interface:bridge.[auto];
 permit src = network:n2; dst = user; prt = tcp 22;
}
END

$out = <<'END';
--r1
! n2_in
access-list n2_in extended permit tcp 10.1.2.0 255.255.255.0 host 10.1.1.1 eq 22
access-list n2_in extended deny ip any any
access-group n2_in in interface n2
--r2
! n2_in
access-list n2_in extended permit tcp 10.1.2.0 255.255.255.0 host 10.1.1.1 eq 22
access-list n2_in extended deny ip any any
access-group n2_in in interface n2
END

test_run($title, $in, $out);

############################################################
done_testing;
